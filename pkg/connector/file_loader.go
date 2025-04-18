package connector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// getColumnIndex finds the 0-based index of a column name in a header row.
// Returns -1 if not found. Case-insensitive comparison.
func getColumnIndex(headers []string, columnName string) int {
	target := strings.ToLower(strings.TrimSpace(columnName))
	for i, h := range headers {
		if strings.ToLower(strings.TrimSpace(h)) == target {
			return i
		}
	}
	return -1
}

// safeGet retrieves a cell value safely, returning an empty string if index is out of bounds.
func safeGet(row []string, headerMap map[string]int, headerName string) string {
	idx, ok := headerMap[headerName]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// The LoadFileData function reads data from the specified input file (Excel, YAML, or JSON).
// It is called by syncer methods to load the complete dataset required for processing.
// The syncer methods require this to get the raw data before building local caches.
// Which ensures each sync operation uses data reflecting the file's state at that moment.
// The implementation detects the file type based on its extension and dispatches to the appropriate parser function.
func LoadFileData(filePath string) (*LoadedData, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".xlsx":
		return loadExcelData(filePath, nil)
	case ".yaml", ".yml":
		return loadYamlData(filePath)
	case ".json":
		return loadJsonData(filePath)
	default:
		return nil, fmt.Errorf("unsupported file type: '%s' for file: %s", ext, filePath)
	}
}

// JSON Loading Logic
// loadJsonData handles the specific logic for reading and parsing .json files.
func loadJsonData(filePath string) (*LoadedData, error) {
	// Read the entire file content
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file %s: %w", filePath, err)
	}

	// Initialize the target struct
	var loadedData LoadedData

	// Unmarshal the JSON data into the struct
	err = json.Unmarshal(jsonData, &loadedData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON data from %s: %w", filePath, err)
	}

	if loadedData.Users == nil && loadedData.Resources == nil {
	}

	return &loadedData, nil
}

// YAML Loading Logic
// loadYamlData handles the specific logic for reading and parsing .yaml or .yml files.
func loadYamlData(filePath string) (*LoadedData, error) {
	// Read the entire file content
	yamlData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", filePath, err)
	}

	// Initialize the target struct
	var loadedData LoadedData

	// Unmarshal the YAML data into the struct
	err = yaml.Unmarshal(yamlData, &loadedData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML data from %s: %w", filePath, err)
	}

	// Basic validation (optional, but good practice)
	// Example: Check if essential slices are non-nil (they will be empty if the key exists but has no items)
	if loadedData.Users == nil && loadedData.Resources == nil {
		// Or return an error, depending on requirements
		// return nil, fmt.Errorf("YAML file %s seems empty or missing required top-level keys (users, resources)", filePath)
	}

	return &loadedData, nil
}

// loadExcelData handles the specific logic for reading and parsing .xlsx files.
func loadExcelData(filePath string, l *zap.Logger) (*LoadedData, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			if l != nil {
				l.Error("failed to close file", zap.Error(err), zap.String("file", filePath))
			}
		}
	}()

	loadedData := &LoadedData{
		Users:        make([]UserData, 0),
		Resources:    make([]ResourceData, 0),
		Entitlements: make([]EntitlementData, 0),
		Grants:       make([]GrantData, 0),
	}

	type sheetConfig struct {
		headers []string // List of required header names
		process func(sheetName string, allRows [][]string, headerMap map[string]int) error
	}

	sheetConfigs := map[string]sheetConfig{
		"users": {
			headers: []string{"Name", "Display Name"}, // Required base headers
			process: func(sheetName string, allRows [][]string, headerMap map[string]int) error {
				for i, row := range allRows {
					if i == 0 {
						continue
					}
					userData := UserData{
						Name:        safeGet(row, headerMap, "Name"),
						DisplayName: safeGet(row, headerMap, "Display Name"),
						Email:       safeGet(row, headerMap, "Email"),
						Status:      safeGet(row, headerMap, "Status"),
						Type:        safeGet(row, headerMap, "Type"),
						Profile:     make(map[string]interface{}),
					}
					if userData.Name == "" {
						if l != nil {
							l.Warn("Skipping user row due to missing required field(s)", zap.Int("row_index", i+1), zap.Any("row_data", userData))
						}
						continue
					}

					for header := range headerMap {
						if strings.HasPrefix(header, "Profile: ") {
							profileKey := strings.TrimSpace(strings.TrimPrefix(header, "Profile: "))
							if profileKey != "" {
								profileValue := safeGet(row, headerMap, header)
								if profileValue != "" {
									userData.Profile[strings.ToLower(profileKey)] = profileValue
								}
							}
						}
					}

					loadedData.Users = append(loadedData.Users, userData)
				}
				return nil
			},
		},
		"resources": {
			headers: []string{"Resource Type", "Resource Function", "Name", "Display Name"}, // Required base headers
			process: func(sheetName string, allRows [][]string, headerMap map[string]int) error {
				for i, row := range allRows {
					if i == 0 {
						continue
					}
					resourceData := ResourceData{
						ResourceType:     safeGet(row, headerMap, "Resource Type"),
						ResourceFunction: safeGet(row, headerMap, "Resource Function"),
						Name:             safeGet(row, headerMap, "Name"),
						DisplayName:      safeGet(row, headerMap, "Display Name"),
						Description:      safeGet(row, headerMap, "Description"),
						ParentResource:   safeGet(row, headerMap, "Parent Resource"),
					}
					if resourceData.Name == "" || resourceData.ResourceType == "" || resourceData.ResourceFunction == "" {
						if l != nil {
							l.Warn("Skipping resource row due to missing required field(s)", zap.Int("row_index", i+1), zap.Any("row_data", resourceData))
						}
						continue
					}
					loadedData.Resources = append(loadedData.Resources, resourceData)
				}
				return nil
			},
		},
		"entitlements": {
			headers: []string{"Resource Name", "Entitlement", "Entitlement Display Name"}, // Required base headers
			process: func(sheetName string, allRows [][]string, headerMap map[string]int) error {
				for i, row := range allRows {
					if i == 0 {
						continue
					}
					entitlementData := EntitlementData{
						ResourceName: safeGet(row, headerMap, "Resource Name"),
						Entitlement:  safeGet(row, headerMap, "Entitlement"),
						DisplayName:  safeGet(row, headerMap, "Entitlement Display Name"),
						Description:  safeGet(row, headerMap, "Entitlement Description"),
					}
					if entitlementData.ResourceName == "" || entitlementData.Entitlement == "" {
						if l != nil {
							l.Warn("Skipping entitlement row due to missing required field(s)", zap.Int("row_index", i+1), zap.Any("row_data", entitlementData))
						}
						continue
					}
					loadedData.Entitlements = append(loadedData.Entitlements, entitlementData)
				}
				return nil
			},
		},
		"grants": {
			headers: []string{"Principal Receiving Grant", "Entitlement Granted to Prinicpal"}, // Required headers (using typo as seen)
			process: func(sheetName string, allRows [][]string, headerMap map[string]int) error {
				for i, row := range allRows {
					if i == 0 {
						continue
					}
					grantData := GrantData{
						Principal:     safeGet(row, headerMap, "Principal Receiving Grant"),
						EntitlementId: safeGet(row, headerMap, "Entitlement Granted to Prinicpal"),
					}
					if grantData.Principal == "" || grantData.EntitlementId == "" {
						if l != nil {
							l.Warn("Skipping grant row due to missing required field(s)", zap.Int("row_index", i+1), zap.Any("row_data", grantData))
						}
						continue
					}
					loadedData.Grants = append(loadedData.Grants, grantData)
				}
				return nil
			},
		},
	}

	for sheetName, config := range sheetConfigs {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			if l != nil {
				l.Warn("Failed to read sheet, skipping.", zap.String("sheet", sheetName), zap.Error(err))
			}
			continue
		}

		if len(rows) <= 1 {
			if l != nil {
				l.Warn("Sheet has no data rows, skipping.", zap.String("sheet", sheetName))
			}
			continue
		}

		headers := rows[0]
		headerMap := make(map[string]int)
		foundRequired := true
		for _, reqHeader := range config.headers {
			idx := getColumnIndex(headers, reqHeader)
			if idx == -1 {
				if l != nil {
					l.Error("Required column missing in sheet, skipping.", zap.String("sheet", sheetName), zap.String("missing_header", reqHeader))
				}
				foundRequired = false
				break
			}
			headerMap[reqHeader] = idx
		}

		for idx, h := range headers {
			if _, required := headerMap[h]; !required {
				headerMap[h] = idx
			}
		}

		if !foundRequired {
			continue
		}

		err = config.process(sheetName, rows, headerMap)
		if err != nil {
			if l != nil {
				l.Error("Error processing sheet data", zap.String("sheet", sheetName), zap.Error(err))
			}
		}
	}

	if l != nil {
		l.Info("Finished loading data from file",
			zap.Int("user_count", len(loadedData.Users)),
			zap.Int("resource_count", len(loadedData.Resources)),
			zap.Int("entitlement_count", len(loadedData.Entitlements)),
			zap.Int("grant_count", len(loadedData.Grants)),
		)
	}
	return loadedData, nil
}
