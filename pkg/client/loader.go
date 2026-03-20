package client

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadFileData(filePath string) (*LoadedData, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".xlsx":
		return loadExcelData(filePath)
	case ".yaml", ".yml":
		return loadYamlData(filePath)
	case ".json", ".jsonc":
		return loadJSONData(filePath)
	case ".csv":
		return loadCSVData(filePath)
	default:
		return nil, fmt.Errorf("baton-file: unsupported file type %q for file: %s", ext, filePath)
	}
}

const userTypeName = "user"

var deprecatedFieldMappings = map[string]map[string]string{
	"users": {
		"name": "id",
	},
	"resources": {
		"name":              "id",
		"resource_function": "trait",
	},
	"entitlements": {
		"resource_name": "resource_id",
		"entitlement":   "entitlement_slug",
	},
}

// ValidateUniqueIDs checks that no ID appears more than once across users and
// resources. IDs must be globally unique because grants and parent references
// resolve by raw ID — a collision would silently drop data.
func ValidateUniqueIDs(data *LoadedData) error {
	seen := make(map[string]string) // id → userTypeName or resource_type
	for _, u := range data.Users {
		if u.ID == "" {
			continue
		}
		if existing, ok := seen[u.ID]; ok {
			return fmt.Errorf(
				"baton-file: duplicate ID %q — found in both %q and %q. "+
					"IDs must be unique across all users and resources. "+
					"Change one of them to a different value",
				u.ID, existing, userTypeName,
			)
		}
		seen[u.ID] = userTypeName
	}
	for _, r := range data.Resources {
		if r.ID == "" {
			continue
		}
		if existing, ok := seen[r.ID]; ok {
			return fmt.Errorf(
				"baton-file: duplicate ID %q — found in both %q and %q. "+
					"IDs must be unique across all users and resources. "+
					"Change one of them to a different value",
				r.ID, existing, r.ResourceType,
			)
		}
		seen[r.ID] = r.ResourceType
	}
	return nil
}

var validTraits = map[string]bool{
	"user": true, "group": true, "role": true, "app": true, "secret": true,
}

// ValidateTraits checks that every resource has a recognized trait value.
func ValidateTraits(data *LoadedData) error {
	for _, r := range data.Resources {
		if r.Trait == "" {
			return fmt.Errorf(
				"baton-file: resource %q is missing a trait. "+
					"Valid traits are: user, group, role, app, secret",
				r.ID,
			)
		}
		if !validTraits[strings.ToLower(r.Trait)] {
			return fmt.Errorf(
				"baton-file: resource %q has unrecognized trait %q. "+
					"Valid traits are: user, group, role, app, secret",
				r.ID, r.Trait,
			)
		}
	}
	return nil
}

// ValidateEntitlementFields checks that every entitlement has the required
// resource_id and entitlement_slug fields populated.
func ValidateEntitlementFields(data *LoadedData) error {
	for i, e := range data.Entitlements {
		if e.ResourceID == "" {
			return fmt.Errorf(
				"baton-file: entitlement #%d is missing required field \"resource_id\". "+
					"Every entitlement must reference the resource it belongs to",
				i+1,
			)
		}
		if e.EntitlementSlug == "" {
			return fmt.Errorf(
				"baton-file: entitlement #%d (resource_id %q) is missing required field \"entitlement_slug\". "+
					"Every entitlement must have a unique slug (e.g., \"member\", \"admin\")",
				i+1, e.ResourceID,
			)
		}
	}
	return nil
}

// ValidateSecretFields checks that secret-specific fields (created_at,
// expires_at, created_by, identity) are only used on resources with
// the "secret" trait. Using them on other traits has no effect and
// almost always indicates a mistake.
func ValidateSecretFields(data *LoadedData) error {
	for _, r := range data.Resources {
		if strings.ToLower(r.Trait) == "secret" {
			continue
		}
		var badFields []string
		if r.CreatedAt != "" {
			badFields = append(badFields, "created_at")
		}
		if r.ExpiresAt != "" {
			badFields = append(badFields, "expires_at")
		}
		if r.CreatedBy != "" {
			badFields = append(badFields, "created_by")
		}
		if r.Identity != "" {
			badFields = append(badFields, "identity")
		}
		if len(badFields) > 0 {
			return fmt.Errorf(
				"baton-file: resource %q (trait %q) uses secret-specific fields: %s. "+
					"These fields only apply to resources with trait \"secret\". "+
					"Remove them or change the trait to \"secret\"",
				r.ID, r.Trait, strings.Join(badFields, ", "),
			)
		}
	}
	return nil
}

// ValidateReferences checks that every cross-reference between sections points
// to an existing ID. This catches typos and missing definitions at startup
// rather than producing scattered runtime warnings during sync.
func ValidateReferences(data *LoadedData) error {
	userIDs := make(map[string]bool, len(data.Users))
	for _, u := range data.Users {
		if u.ID != "" {
			userIDs[u.ID] = true
		}
	}

	allIDs := make(map[string]bool, len(data.Users)+len(data.Resources))
	for id := range userIDs {
		allIDs[id] = true
	}
	for _, r := range data.Resources {
		if r.ID != "" {
			allIDs[r.ID] = true
		}
	}

	// Entitlement key set for grant validation below.
	entitlementKeys := make(map[string]bool, len(data.Entitlements))
	for _, e := range data.Entitlements {
		if e.ResourceID != "" && e.EntitlementSlug != "" {
			entitlementKeys[e.ResourceID+":"+e.EntitlementSlug] = true
		}
	}

	for _, r := range data.Resources {
		if r.ParentResource != "" && !allIDs[r.ParentResource] {
			return fmt.Errorf(
				"baton-file: resource %q references parent_resource %q, "+
					"but no user or resource with that ID exists. "+
					"Check for typos or add the missing resource",
				r.ID, r.ParentResource,
			)
		}
	}

	// Entitlements can reference user or resource IDs — entitlements on users
	// are uncommon but valid (e.g., impersonation permissions).
	for _, e := range data.Entitlements {
		if e.ResourceID != "" && !allIDs[e.ResourceID] {
			return fmt.Errorf(
				"baton-file: entitlement %q on resource_id %q — "+
					"no user or resource with that ID exists. "+
					"Check for typos or add the missing resource",
				e.EntitlementSlug, e.ResourceID,
			)
		}
	}

	for i, g := range data.DirectUserGrants {
		if g.PrincipalID != "" && !userIDs[g.PrincipalID] {
			return fmt.Errorf(
				"baton-file: direct_user_grant #%d references principal_id %q, "+
					"but no user with that ID exists. "+
					"principal_id must be a user ID, not a resource ID",
				i+1, g.PrincipalID,
			)
		}
		if g.ResourceID != "" && !allIDs[g.ResourceID] {
			return fmt.Errorf(
				"baton-file: direct_user_grant #%d references resource_id %q, "+
					"but no user or resource with that ID exists. "+
					"Check for typos or add the missing resource",
				i+1, g.ResourceID,
			)
		}
		entKey := g.ResourceID + ":" + g.EntitlementSlug
		if g.ResourceID != "" && g.EntitlementSlug != "" && !entitlementKeys[entKey] {
			return fmt.Errorf(
				"baton-file: direct_user_grant #%d references entitlement %q, "+
					"but no entitlement with that resource_id and slug exists. "+
					"Check for typos or add the missing entitlement definition",
				i+1, entKey,
			)
		}
	}

	for i, m := range data.GrantInheritanceMappings {
		if m.PrincipalResourceID != "" && !allIDs[m.PrincipalResourceID] {
			return fmt.Errorf(
				"baton-file: grant_inheritance_mapping #%d references principal_resource_id %q, "+
					"but no user or resource with that ID exists. "+
					"Check for typos or add the missing resource",
				i+1, m.PrincipalResourceID,
			)
		}
		if m.InheritedResourceID != "" && !allIDs[m.InheritedResourceID] {
			return fmt.Errorf(
				"baton-file: grant_inheritance_mapping #%d references inherited_resource_id %q, "+
					"but no user or resource with that ID exists. "+
					"Check for typos or add the missing resource",
				i+1, m.InheritedResourceID,
			)
		}
		principalEntKey := m.PrincipalResourceID + ":" + m.PrincipalEntitlementSlug
		if m.PrincipalResourceID != "" && m.PrincipalEntitlementSlug != "" && !entitlementKeys[principalEntKey] {
			return fmt.Errorf(
				"baton-file: grant_inheritance_mapping #%d references principal entitlement %q, "+
					"but no entitlement with that resource_id and slug exists. "+
					"Check for typos or add the missing entitlement definition",
				i+1, principalEntKey,
			)
		}
		inheritedEntKey := m.InheritedResourceID + ":" + m.InheritedEntitlementSlug
		if m.InheritedResourceID != "" && m.InheritedEntitlementSlug != "" && !entitlementKeys[inheritedEntKey] {
			return fmt.Errorf(
				"baton-file: grant_inheritance_mapping #%d references inherited entitlement %q, "+
					"but no entitlement with that resource_id and slug exists. "+
					"Check for typos or add the missing entitlement definition",
				i+1, inheritedEntKey,
			)
		}
		if m.InheritanceDepth != "full" && m.InheritanceDepth != "shallow" {
			return fmt.Errorf(
				"baton-file: grant_inheritance_mapping #%d has invalid inheritance_depth %q. "+
					"Must be \"full\" or \"shallow\"",
				i+1, m.InheritanceDepth,
			)
		}
	}

	return nil
}

func ValidateLoadedData(rawBytes []byte, format string) error {
	var raw map[string]interface{}

	switch format {
	case "yaml":
		if err := yaml.Unmarshal(rawBytes, &raw); err != nil {
			return fmt.Errorf("baton-file: failed to parse %s for validation: %w", format, err)
		}
	case "json":
		if err := json.Unmarshal(rawBytes, &raw); err != nil {
			return fmt.Errorf("baton-file: failed to parse %s for validation: %w", format, err)
		}
	default:
		return nil
	}

	if _, ok := raw["grants"]; ok {
		return fmt.Errorf("baton-file: \"grants\" section is no longer supported, use \"direct_user_grants\" and \"grant_inheritance_mappings\" instead")
	}

	for sectionName, fieldMap := range deprecatedFieldMappings {
		sectionRaw, ok := raw[sectionName]
		if !ok {
			continue
		}
		records, ok := sectionRaw.([]interface{})
		if !ok {
			continue
		}
		for _, record := range records {
			recordMap, ok := record.(map[string]interface{})
			if !ok {
				continue
			}
			for oldName, newName := range fieldMap {
				if _, found := recordMap[oldName]; found {
					return fmt.Errorf(
						"baton-file: field %q is no longer supported in %s section, use %q instead",
						oldName, sectionName, newName,
					)
				}
			}
		}
	}

	return nil
}

