package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/conductorone/baton-file/pkg/client"
	"github.com/conductorone/baton-file/pkg/connector"
)

func TestLoadYAML(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.yaml")
	if err != nil {
		t.Fatalf("failed to load yaml: %v", err)
	}
	assertFullExampleData(t, data)
}

func TestLoadJSON(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.json")
	if err != nil {
		t.Fatalf("failed to load json: %v", err)
	}
	assertFullExampleData(t, data)
}

func TestLoadCSV(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.csv")
	if err != nil {
		t.Fatalf("failed to load csv: %v", err)
	}
	assertFullExampleData(t, data)
}

func TestMinimalYAML(t *testing.T) {
	data, err := client.LoadFileData("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("failed to load minimal yaml: %v", err)
	}
	assertEqual(t, "users count", 1, len(data.Users))
	assertEqual(t, "resources count", 1, len(data.Resources))
	assertEqual(t, "entitlements count", 1, len(data.Entitlements))
	assertEqual(t, "direct grants count", 1, len(data.DirectUserGrants))
	assertEqual(t, "inheritance mappings count", 0, len(data.GrantInheritanceMappings))
}

func TestUsersOnly(t *testing.T) {
	data, err := client.LoadFileData("testdata/users-only.yaml")
	if err != nil {
		t.Fatalf("failed to load users-only yaml: %v", err)
	}
	assertEqual(t, "users count", 2, len(data.Users))
	assertEqual(t, "resources count", 0, len(data.Resources))
	assertEqual(t, "entitlements count", 0, len(data.Entitlements))
	assertEqual(t, "direct grants count", 0, len(data.DirectUserGrants))
}

func TestDeprecatedFieldDetection(t *testing.T) {
	yamlWithOldName := "users:\n  - name: alice\n    display_name: Alice\n"
	tmpFile := t.TempDir() + "/old.yaml"
	if err := os.WriteFile(tmpFile, []byte(yamlWithOldName), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := client.LoadFileData(tmpFile)
	if err == nil {
		t.Fatal("expected error for deprecated 'name' field, got nil")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("expected 'no longer supported' in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("expected 'id' in error, got: %s", err.Error())
	}
}

func TestDeprecatedGrantsSection(t *testing.T) {
	yamlWithGrants := "users: []\ngrants:\n  - principal: alice\n    entitlement_id: \"team:member\"\n"
	tmpFile := t.TempDir() + "/old-grants.yaml"
	if err := os.WriteFile(tmpFile, []byte(yamlWithGrants), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := client.LoadFileData(tmpFile)
	if err == nil {
		t.Fatal("expected error for deprecated 'grants' section, got nil")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("expected 'no longer supported' in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "direct_user_grants") {
		t.Errorf("expected 'direct_user_grants' in error, got: %s", err.Error())
	}
}

func TestFullSyncPipeline(t *testing.T) {
	for _, format := range []string{"yaml", "json", "csv"} {
		t.Run(format, func(t *testing.T) {
			data, err := client.LoadFileData("testdata/full-example." + format)
			if err != nil {
				t.Fatalf("load failed: %v", err)
			}
			types := discoverResourceTypes(data)
			if len(types) != 6 {
				t.Errorf("expected 6 resource types, got %d: %v", len(types), types)
			}
		})
	}
}

func TestUserTraitFields(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.yaml")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	jane := data.Users[0]
	assertEqual(t, "jane id", "jane.smith", jane.ID)
	assertEqual(t, "jane email", "jane.smith@example.com", jane.Email)
	assertEqual(t, "jane login", "jsmith", jane.Login)
	assertEqual(t, "jane employee_id count", 1, len(jane.EmployeeID))
	assertEqual(t, "jane login_aliases count", 2, len(jane.LoginAliases))
	if jane.MFAEnabled == nil || !*jane.MFAEnabled {
		t.Error("jane.MFAEnabled should be true")
	}
	if jane.SSOEnabled == nil || !*jane.SSOEnabled {
		t.Error("jane.SSOEnabled should be true")
	}
	assertEqual(t, "jane status_details", "Active since 2021", jane.StatusDetails)

	bob := data.Users[1]
	assertEqual(t, "bob employee_id count", 2, len(bob.EmployeeID))
	if bob.MFAEnabled == nil || *bob.MFAEnabled {
		t.Error("bob.MFAEnabled should be false")
	}
}

func TestSecretTraitFields(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.yaml")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	var secret *client.ResourceData
	for i := range data.Resources {
		if data.Resources[i].ID == "deploy-token-1" {
			secret = &data.Resources[i]
			break
		}
	}
	if secret == nil {
		t.Fatal("deploy-token-1 not found")
	}
	assertEqual(t, "trait", "secret", secret.Trait)
	assertEqual(t, "created_at", "2025-01-10T00:00:00Z", secret.CreatedAt)
	assertEqual(t, "expires_at", "2026-01-10T00:00:00Z", secret.ExpiresAt)
	assertEqual(t, "created_by", "user:jane.smith", secret.CreatedBy)
	assertEqual(t, "identity", "repository:api-service", secret.Identity)
}

func TestResourceProfiles(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.yaml")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	var org *client.ResourceData
	for i := range data.Resources {
		if data.Resources[i].ID == "acme-org" {
			org = &data.Resources[i]
			break
		}
	}
	if org == nil {
		t.Fatal("acme-org not found")
	}
	if org.Profile == nil {
		t.Fatal("acme-org profile should not be nil")
	}
	if org.Profile["region"] != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %v", org.Profile["region"])
	}
}

func TestGrantInheritanceDepthValues(t *testing.T) {
	data, err := client.LoadFileData("testdata/full-example.yaml")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	assertEqual(t, "inheritance mappings count", 2, len(data.GrantInheritanceMappings))
	assertEqual(t, "first depth", "full", data.GrantInheritanceMappings[0].InheritanceDepth)
	assertEqual(t, "second depth", "shallow", data.GrantInheritanceMappings[1].InheritanceDepth)
}

func TestJSONCWithComments(t *testing.T) {
	jsonc := `{
		// This is a line comment
		"users": [
			{
				"id": "test.user", /* inline block comment */
				"display_name": "Test User",
			}
		],
		"resources": [],
		"entitlements": [],
		"direct_user_grants": [],
	}`
	tmpFile := t.TempDir() + "/test.jsonc"
	if err := os.WriteFile(tmpFile, []byte(jsonc), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := client.LoadFileData(tmpFile)
	if err != nil {
		t.Fatalf("failed to load jsonc: %v", err)
	}
	assertEqual(t, "users count", 1, len(data.Users))
	assertEqual(t, "user id", "test.user", data.Users[0].ID)
	assertEqual(t, "user display_name", "Test User", data.Users[0].DisplayName)
}

func TestQuickstartYAML(t *testing.T) {
	data, err := client.LoadFileData("../../templates/baton-file-yaml-quickstart-template.yaml")
	if err != nil {
		t.Fatalf("failed to load quickstart yaml: %v", err)
	}
	assertEqual(t, "users", 2, len(data.Users))
	assertEqual(t, "resources", 2, len(data.Resources))
	assertEqual(t, "entitlements", 2, len(data.Entitlements))
	assertEqual(t, "direct_user_grants", 2, len(data.DirectUserGrants))
	assertEqual(t, "grant_inheritance_mappings", 0, len(data.GrantInheritanceMappings))
}

func TestQuickstartJSONC(t *testing.T) {
	data, err := client.LoadFileData("../../templates/baton-file-jsonc-quickstart-template.jsonc")
	if err != nil {
		t.Fatalf("failed to load quickstart jsonc: %v", err)
	}
	assertEqual(t, "users", 2, len(data.Users))
	assertEqual(t, "resources", 2, len(data.Resources))
	assertEqual(t, "entitlements", 2, len(data.Entitlements))
	assertEqual(t, "direct_user_grants", 2, len(data.DirectUserGrants))
}

func TestQuickstartCSV(t *testing.T) {
	data, err := client.LoadFileData("../../templates/baton-file-csv-quickstart-template.csv")
	if err != nil {
		t.Fatalf("failed to load quickstart csv: %v", err)
	}
	assertEqual(t, "users", 2, len(data.Users))
	assertEqual(t, "resources", 2, len(data.Resources))
	assertEqual(t, "entitlements", 2, len(data.Entitlements))
	assertEqual(t, "direct_user_grants", 2, len(data.DirectUserGrants))
}

func TestFullJSONCTemplate(t *testing.T) {
	data, err := client.LoadFileData("../../templates/baton-file-jsonc-template.jsonc")
	if err != nil {
		t.Fatalf("failed to load full jsonc template: %v", err)
	}
	assertFullExampleData(t, data)
}

// --- helpers ---

func assertFullExampleData(t *testing.T, data *client.LoadedData) {
	t.Helper()
	assertEqual(t, "users", 3, len(data.Users))
	assertEqual(t, "resources", 5, len(data.Resources))
	assertEqual(t, "entitlements", 4, len(data.Entitlements))
	assertEqual(t, "direct_user_grants", 4, len(data.DirectUserGrants))
	assertEqual(t, "grant_inheritance_mappings", 2, len(data.GrantInheritanceMappings))

	assertEqual(t, "user[0].id", "jane.smith", data.Users[0].ID)
	assertEqual(t, "user[1].id", "john.doe", data.Users[1].ID)
	assertEqual(t, "user[2].id", "maria.garcia", data.Users[2].ID)
}

func assertEqual(t *testing.T, name string, expected, actual interface{}) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s: expected %v, got %v", name, expected, actual)
	}
}

func discoverResourceTypes(data *client.LoadedData) map[string]bool {
	types := make(map[string]bool)
	if len(data.Users) > 0 {
		types["user"] = true
	}
	for _, r := range data.Resources {
		rt := strings.ToLower(r.ResourceType)
		types[rt] = true
	}
	return types
}

var (
	_ = context.Background
	_ = connector.TraitMap
)
