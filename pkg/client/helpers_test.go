package client

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseTime_RFC3339(t *testing.T) {
	got, err := ParseTime("2025-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2025 || got.Month() != 3 || got.Day() != 15 {
		t.Errorf("unexpected date: %v", got)
	}
}

func TestParseTime_ISODate(t *testing.T) {
	got, err := ParseTime("2025-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2025 {
		t.Errorf("unexpected year: %d", got.Year())
	}
}

func TestParseTime_USDate(t *testing.T) {
	got, err := ParseTime("03/15/2025")
	if err != nil {
		t.Fatal(err)
	}
	if got.Month() != 3 || got.Day() != 15 {
		t.Errorf("unexpected date: %v", got)
	}
}

func TestParseTime_UnixSeconds(t *testing.T) {
	got, err := ParseTime("1710496200")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() < 2024 {
		t.Errorf("unexpected year: %d", got.Year())
	}
}

func TestParseTime_UnixMillis(t *testing.T) {
	got, err := ParseTime("1710496200000")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() < 2024 {
		t.Errorf("unexpected year: %d", got.Year())
	}
}

func TestParseTime_Empty(t *testing.T) {
	_, err := ParseTime("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParseTime_Invalid(t *testing.T) {
	_, err := ParseTime("not-a-date")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestFlexibleStringList_YAMLSingle(t *testing.T) {
	var f FlexibleStringList
	if err := yaml.Unmarshal([]byte(`"hello"`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f) != 1 || f[0] != "hello" {
		t.Errorf("expected [hello], got %v", f)
	}
}

func TestFlexibleStringList_YAMLList(t *testing.T) {
	var f FlexibleStringList
	if err := yaml.Unmarshal([]byte("- a\n- b"), &f); err != nil {
		t.Fatal(err)
	}
	if len(f) != 2 || f[0] != "a" || f[1] != "b" {
		t.Errorf("expected [a b], got %v", f)
	}
}

func TestFlexibleStringList_YAMLEmpty(t *testing.T) {
	var f FlexibleStringList
	if err := yaml.Unmarshal([]byte(`""`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f) != 0 {
		t.Errorf("expected empty, got %v", f)
	}
}

func TestFlexibleStringList_JSONSingle(t *testing.T) {
	var f FlexibleStringList
	if err := json.Unmarshal([]byte(`"hello"`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f) != 1 || f[0] != "hello" {
		t.Errorf("expected [hello], got %v", f)
	}
}

func TestFlexibleStringList_JSONArray(t *testing.T) {
	var f FlexibleStringList
	if err := json.Unmarshal([]byte(`["a","b"]`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f) != 2 {
		t.Errorf("expected 2 items, got %v", f)
	}
}

func TestFlexibleStringList_JSONNull(t *testing.T) {
	var f FlexibleStringList
	if err := json.Unmarshal([]byte(`null`), &f); err != nil {
		t.Fatal(err)
	}
	if f != nil {
		t.Errorf("expected nil, got %v", f)
	}
}

func TestParseTypeColonID_Valid(t *testing.T) {
	rt, id, err := ParseTypeColonID("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	if rt != "user" || id != "alice" {
		t.Errorf("expected user/alice, got %s/%s", rt, id)
	}
}

func TestParseTypeColonID_MultipleColons(t *testing.T) {
	rt, id, err := ParseTypeColonID("type:id:with:colons")
	if err != nil {
		t.Fatal(err)
	}
	if rt != "type" || id != "id:with:colons" {
		t.Errorf("expected type/id:with:colons, got %s/%s", rt, id)
	}
}

func TestParseTypeColonID_MissingColon(t *testing.T) {
	_, _, err := ParseTypeColonID("nocolon")
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
}

func TestParseTypeColonID_Empty(t *testing.T) {
	_, _, err := ParseTypeColonID("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}
