package client

import (
	"encoding/json"
	"testing"
)

func TestStripJSONComments_LineComments(t *testing.T) {
	input := `{
		// this is a comment
		"key": "value"
	}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
	if m["key"] != "value" {
		t.Errorf("expected key=value, got %v", m["key"])
	}
}

func TestStripJSONComments_BlockComments(t *testing.T) {
	input := `{
		/* block comment */
		"key": "value" /* inline */
	}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
	if m["key"] != "value" {
		t.Errorf("expected key=value, got %v", m["key"])
	}
}

func TestStripJSONComments_TrailingCommas(t *testing.T) {
	input := `{
		"arr": [1, 2, 3,],
		"key": "value",
	}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
	arr := m["arr"].([]interface{})
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr))
	}
}

func TestStripJSONComments_CommentsInStringsPreserved(t *testing.T) {
	input := `{"msg": "has // slashes and /* stars */"}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
	if m["msg"] != "has // slashes and /* stars */" {
		t.Errorf("string content was corrupted: %v", m["msg"])
	}
}

func TestStripJSONComments_EscapedQuotesInStrings(t *testing.T) {
	input := `{"msg": "escaped \" quote // not a comment"}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
	expected := `escaped " quote // not a comment`
	if m["msg"] != expected {
		t.Errorf("expected %q, got %q", expected, m["msg"])
	}
}

func TestStripJSONComments_Combined(t *testing.T) {
	input := `{
		// top-level comment
		"users": [
			{
				"id": "test", /* inline */
				"name": "Test User",  // trailing comment
			},
		],
	}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
	users := m["users"].([]interface{})
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
	user := users[0].(map[string]interface{})
	if user["id"] != "test" {
		t.Errorf("expected id=test, got %v", user["id"])
	}
}

func TestStripJSONComments_NoComments(t *testing.T) {
	input := `{"key": "value", "num": 42}`
	out := stripJSONComments([]byte(input))
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json after stripping: %v\noutput: %s", err, out)
	}
}
