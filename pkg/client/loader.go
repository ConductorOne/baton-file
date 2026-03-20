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

