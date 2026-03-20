package client

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// No deprecated field detection — CSV was added in this version with the
// current field names. There are no legacy CSV files to migrate from.
func loadCSVData(filePath string) (*LoadedData, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("baton-file: failed to open csv file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("baton-file: failed to read csv: %w", err)
	}
	if len(records) < 2 {
		return &LoadedData{}, nil
	}

	header := records[0]
	headerMap := make(map[string]int)
	var profileColumns []string
	for i, h := range header {
		normalized := strings.TrimSpace(h)
		headerMap[normalized] = i
		if strings.HasPrefix(strings.ToLower(normalized), "profile.") {
			profileColumns = append(profileColumns, normalized)
		}
	}

	if _, ok := headerMap["record_type"]; !ok {
		return nil, fmt.Errorf("baton-file: csv missing required \"record_type\" column")
	}

	csvGet := func(row []string, col string) string {
		idx, exists := headerMap[col]
		if !exists || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	csvSplitList := func(row []string, col string) FlexibleStringList {
		return SplitCommaSeparated(csvGet(row, col))
	}

	csvParseBool := func(row []string, col string) *bool {
		return ParseOptionalBool(csvGet(row, col))
	}

	csvBuildProfile := func(row []string) map[string]interface{} {
		profile := make(map[string]interface{})
		for _, col := range profileColumns {
			val := csvGet(row, col)
			if val != "" {
				key := strings.TrimPrefix(strings.ToLower(col), "profile.")
				profile[key] = val
			}
		}
		if len(profile) == 0 {
			return nil
		}
		return profile
	}

	data := &LoadedData{}

	for _, row := range records[1:] {
		recordType := csvGet(row, "record_type")
		if recordType == "" {
			continue
		}

		switch strings.ToLower(recordType) {
		case "user":
			data.Users = append(data.Users, UserData{
				ID:               csvGet(row, "id"),
				DisplayName:      csvGet(row, "display_name"),
				Email:            csvGet(row, "email"),
				Status:           csvGet(row, "status"),
				Type:             csvGet(row, "type"),
				LastLogin:        csvGet(row, "last_login"),
				Login:            csvGet(row, "login"),
				LoginAliases:     csvSplitList(row, "login_aliases"),
				EmployeeID:       csvSplitList(row, "employee_id"),
				AdditionalEmails: csvSplitList(row, "additional_emails"),
				MFAEnabled:       csvParseBool(row, "mfa_enabled"),
				SSOEnabled:       csvParseBool(row, "sso_enabled"),
				StatusDetails:    csvGet(row, "status_details"),
				Profile:          csvBuildProfile(row),
			})
		case "resource":
			data.Resources = append(data.Resources, ResourceData{
				ResourceType:   csvGet(row, "resource_type"),
				Trait:          csvGet(row, "trait"),
				ID:             csvGet(row, "id"),
				DisplayName:    csvGet(row, "display_name"),
				Description:    csvGet(row, "description"),
				ParentResource: csvGet(row, "parent_resource"),
				Profile:        csvBuildProfile(row),
				CreatedAt:      csvGet(row, "created_at"),
				ExpiresAt:      csvGet(row, "expires_at"),
				CreatedBy:      csvGet(row, "created_by"),
				Identity:       csvGet(row, "identity"),
			})
		case "entitlement":
			data.Entitlements = append(data.Entitlements, EntitlementData{
				ResourceID:      csvGet(row, "resource_id"),
				EntitlementSlug: csvGet(row, "entitlement_slug"),
				DisplayName:     csvGet(row, "display_name"),
				Description:     csvGet(row, "description"),
			})
		case "direct_user_grant":
			data.DirectUserGrants = append(data.DirectUserGrants, DirectUserGrant{
				PrincipalID:     csvGet(row, "principal_id"),
				ResourceID:      csvGet(row, "resource_id"),
				EntitlementSlug: csvGet(row, "entitlement_slug"),
			})
		case "grant_inheritance_mapping":
			data.GrantInheritanceMappings = append(data.GrantInheritanceMappings, GrantInheritanceMapping{
				PrincipalResourceID:      csvGet(row, "principal_resource_id"),
				PrincipalEntitlementSlug: csvGet(row, "principal_entitlement_slug"),
				InheritedResourceID:      csvGet(row, "inherited_resource_id"),
				InheritedEntitlementSlug: csvGet(row, "inherited_entitlement_slug"),
				InheritanceDepth:         csvGet(row, "inheritance_depth"),
			})
		default:
			continue
		}
	}

	return data, nil
}
