package client

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func getColumnIndex(headers []string, columnName string) int {
	target := strings.ToLower(strings.TrimSpace(columnName))
	for i, h := range headers {
		if strings.ToLower(strings.TrimSpace(h)) == target {
			return i
		}
	}
	return -1
}

func safeGet(row []string, headerMap map[string]int, headerName string) string {
	idx, ok := headerMap[strings.ToLower(strings.TrimSpace(headerName))]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func buildHeaderMap(headers []string) map[string]int {
	m := make(map[string]int)
	for i, h := range headers {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// Thin wrappers adapting the XLSX row+headerMap lookup to shared helpers.
// CSV has equivalent wrappers with a different call signature (closure over headerMap).
func xlsxSplitList(row []string, headerMap map[string]int, headerName string) FlexibleStringList {
	return SplitCommaSeparated(safeGet(row, headerMap, headerName))
}

func xlsxParseBool(row []string, headerMap map[string]int, headerName string) *bool {
	return ParseOptionalBool(safeGet(row, headerMap, headerName))
}

// detectOldHeaders checks for deprecated column names from the previous XLSX
// template. Matching is case-sensitive because the old template always used
// Title Case headers; buildHeaderMap/safeGet handle case-insensitive lookup
// for current column names separately.
func detectOldHeaders(headers []string, oldToNew map[string]string) error {
	for _, h := range headers {
		normalized := strings.TrimSpace(h)
		if newName, ok := oldToNew[normalized]; ok {
			return fmt.Errorf("baton-file: xlsx uses old header %q, use %q instead", normalized, newName)
		}
	}
	return nil
}

// findSheetRows tries each name in order and returns the first sheet found
// with data rows. This allows Title Case sheet names (preferred) with a
// lowercase fallback for backward compatibility.
func findSheetRows(f *excelize.File, names ...string) ([][]string, bool) {
	for _, name := range names {
		rows, err := f.GetRows(name)
		if err == nil && len(rows) > 1 {
			return rows, true
		}
	}
	return nil, false
}

func loadExcelData(filePath string) (*LoadedData, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("baton-file: failed to open xlsx file: %w", err)
	}
	defer f.Close()

	for _, name := range f.GetSheetList() {
		if strings.ToLower(strings.TrimSpace(name)) == "grants" {
			return nil, fmt.Errorf("baton-file: xlsx \"Grants\" sheet is no longer supported, use \"Direct User Grants\" and \"Grant Inheritance Mappings\" sheets")
		}
	}

	loadedData := &LoadedData{}

	// --- Users sheet ---
	if rows, ok := findSheetRows(f, "Users", "users"); ok {
		headers := rows[0]
		if err := detectOldHeaders(headers, map[string]string{"Name": "ID"}); err != nil {
			if getColumnIndex(headers, "ID") == -1 {
				return nil, err
			}
		}
		headerMap := buildHeaderMap(headers)
		for _, row := range rows[1:] {
			userData := UserData{
				ID:               safeGet(row, headerMap, "ID"),
				DisplayName:      safeGet(row, headerMap, "Display Name"),
				Email:            safeGet(row, headerMap, "Email"),
				Status:           safeGet(row, headerMap, "Status"),
				Type:             safeGet(row, headerMap, "Type"),
				LastLogin:        safeGet(row, headerMap, "Last Login"),
				Login:            safeGet(row, headerMap, "Login"),
				LoginAliases:     xlsxSplitList(row, headerMap, "Login Aliases"),
				EmployeeID:       xlsxSplitList(row, headerMap, "Employee ID"),
				AdditionalEmails: xlsxSplitList(row, headerMap, "Additional Emails"),
				MFAEnabled:       xlsxParseBool(row, headerMap, "MFA Enabled"),
				SSOEnabled:       xlsxParseBool(row, headerMap, "SSO Enabled"),
				StatusDetails:    safeGet(row, headerMap, "Status Details"),
				Profile:          make(map[string]interface{}),
			}

			if userData.ID == "" {
				continue
			}

			for header := range headerMap {
				if strings.HasPrefix(header, "profile: ") {
					profileKey := strings.TrimSpace(strings.TrimPrefix(header, "profile: "))
					if profileKey != "" {
						profileValue := safeGet(row, headerMap, header)
						if profileValue != "" {
							userData.Profile[profileKey] = profileValue
						}
					}
				}
			}
			if len(userData.Profile) == 0 {
				userData.Profile = nil
			}

			loadedData.Users = append(loadedData.Users, userData)
		}
	}

	// --- Resources sheet ---
	if rows, ok := findSheetRows(f, "Resources", "resources"); ok {
		headers := rows[0]
		oldResourceHeaders := map[string]string{
			"Name":              "ID",
			"Resource Function": "Trait",
		}
		if err := detectOldHeaders(headers, oldResourceHeaders); err != nil {
			hasNewID := getColumnIndex(headers, "ID") != -1
			hasNewTrait := getColumnIndex(headers, "Trait") != -1
			if !hasNewID || !hasNewTrait {
				return nil, err
			}
		}
		headerMap := buildHeaderMap(headers)

		var profileCols []string
		for key := range headerMap {
			if strings.HasPrefix(key, "profile: ") {
				profileCols = append(profileCols, key)
			}
		}

		for _, row := range rows[1:] {
			resourceData := ResourceData{
				ResourceType:   safeGet(row, headerMap, "Resource Type"),
				Trait:          safeGet(row, headerMap, "Trait"),
				ID:             safeGet(row, headerMap, "ID"),
				DisplayName:    safeGet(row, headerMap, "Display Name"),
				Description:    safeGet(row, headerMap, "Description"),
				ParentResource: safeGet(row, headerMap, "Parent Resource"),
				CreatedAt:      safeGet(row, headerMap, "Created At"),
				ExpiresAt:      safeGet(row, headerMap, "Expires At"),
				CreatedBy:      safeGet(row, headerMap, "Created By"),
				Identity:       safeGet(row, headerMap, "Identity"),
			}

			if resourceData.ID == "" || resourceData.ResourceType == "" || resourceData.Trait == "" {
				continue
			}

			profile := make(map[string]interface{})
			for _, col := range profileCols {
				val := safeGet(row, headerMap, col)
				if val != "" {
					key := strings.TrimSpace(strings.TrimPrefix(col, "profile: "))
					profile[key] = val
				}
			}
			if len(profile) > 0 {
				resourceData.Profile = profile
			}

			loadedData.Resources = append(loadedData.Resources, resourceData)
		}
	}

	// --- Entitlements sheet ---
	if rows, ok := findSheetRows(f, "Entitlements", "entitlements"); ok {
		headers := rows[0]
		oldEntHeaders := map[string]string{
			"Resource Name": "Resource ID",
			"Entitlement":   "Entitlement Slug",
		}
		if err := detectOldHeaders(headers, oldEntHeaders); err != nil {
			hasNewResID := getColumnIndex(headers, "Resource ID") != -1
			hasNewSlug := getColumnIndex(headers, "Entitlement Slug") != -1
			if !hasNewResID || !hasNewSlug {
				return nil, err
			}
		}
		headerMap := buildHeaderMap(headers)
		for _, row := range rows[1:] {
			entData := EntitlementData{
				ResourceID:      safeGet(row, headerMap, "Resource ID"),
				EntitlementSlug: safeGet(row, headerMap, "Entitlement Slug"),
				DisplayName:     safeGet(row, headerMap, "Display Name"),
				Description:     safeGet(row, headerMap, "Description"),
			}

			if entData.ResourceID == "" || entData.EntitlementSlug == "" {
				continue
			}

			loadedData.Entitlements = append(loadedData.Entitlements, entData)
		}
	}

	// --- Direct User Grants sheet ---
	if rows, ok := findSheetRows(f, "Direct User Grants", "direct_user_grants"); ok {
		headerMap := buildHeaderMap(rows[0])
		for _, row := range rows[1:] {
			loadedData.DirectUserGrants = append(loadedData.DirectUserGrants, DirectUserGrant{
				PrincipalID:     safeGet(row, headerMap, "Principal ID"),
				ResourceID:      safeGet(row, headerMap, "Resource ID"),
				EntitlementSlug: safeGet(row, headerMap, "Entitlement Slug"),
			})
		}
	}

	// --- Grant Inheritance Mappings sheet ---
	if rows, ok := findSheetRows(f, "Grant Inheritance Mappings", "grant_inheritance_mappings"); ok {
		headerMap := buildHeaderMap(rows[0])
		for _, row := range rows[1:] {
			loadedData.GrantInheritanceMappings = append(loadedData.GrantInheritanceMappings, GrantInheritanceMapping{
				PrincipalResourceID:      safeGet(row, headerMap, "Principal Resource ID"),
				PrincipalEntitlementSlug: safeGet(row, headerMap, "Principal Entitlement Slug"),
				InheritedResourceID:      safeGet(row, headerMap, "Inherited Resource ID"),
				InheritedEntitlementSlug: safeGet(row, headerMap, "Inherited Entitlement Slug"),
				InheritanceDepth:         safeGet(row, headerMap, "Inheritance Depth"),
			})
		}
	}

	return loadedData, nil
}
