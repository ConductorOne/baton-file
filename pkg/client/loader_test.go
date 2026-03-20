package client

import (
	"os"
	"strings"
	"testing"
)

func TestValidateLoadedData_YAMLDeprecatedName(t *testing.T) {
	input := []byte(`{"users": [{"name": "alice"}]}`)
	err := ValidateLoadedData(input, "json")
	if err == nil {
		t.Fatal("expected error for deprecated 'name' field")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("expected 'no longer supported' in error: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "\"id\"") {
		t.Errorf("expected '\"id\"' in error: %s", err.Error())
	}
}

func TestValidateLoadedData_DeprecatedResourceFunction(t *testing.T) {
	input := []byte(`resources:
  - resource_function: group
    id: team1
`)
	err := ValidateLoadedData(input, "yaml")
	if err == nil {
		t.Fatal("expected error for deprecated 'resource_function' field")
	}
	if !strings.Contains(err.Error(), "trait") {
		t.Errorf("expected 'trait' in error: %s", err.Error())
	}
}

func TestValidateLoadedData_DeprecatedGrants(t *testing.T) {
	input := []byte(`{"grants": [], "users": []}`)
	err := ValidateLoadedData(input, "json")
	if err == nil {
		t.Fatal("expected error for deprecated 'grants' section")
	}
	if !strings.Contains(err.Error(), "direct_user_grants") {
		t.Errorf("expected 'direct_user_grants' in error: %s", err.Error())
	}
}

func TestValidateLoadedData_ValidData(t *testing.T) {
	input := []byte(`{"users": [{"id": "alice"}], "resources": [{"id": "team", "trait": "group"}]}`)
	err := ValidateLoadedData(input, "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFileData_UnsupportedExtension(t *testing.T) {
	tmpFile := t.TempDir() + "/data.txt"
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFileData(tmpFile)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error: %s", err.Error())
	}
}
