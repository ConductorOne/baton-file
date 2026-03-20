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

