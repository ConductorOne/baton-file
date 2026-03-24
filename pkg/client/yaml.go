package client

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func loadYamlData(filePath string) (*LoadedData, error) {
	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("baton-file: failed to read yaml file: %w", err)
	}

	if err := ValidateLoadedData(rawBytes, "yaml"); err != nil {
		return nil, err
	}

	var data LoadedData
	if err := yaml.Unmarshal(rawBytes, &data); err != nil {
		return nil, fmt.Errorf("baton-file: failed to unmarshal yaml: %w", err)
	}

	return &data, nil
}
