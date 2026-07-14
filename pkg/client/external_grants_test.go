package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// externalGrantBase returns LoadedData with a valid resource + entitlement to
// hang external grants off of.
func externalGrantBase(grants ...ExternalGrant) *LoadedData {
	return &LoadedData{
		Resources: []ResourceData{
			{ID: "payroll-app", ResourceType: "app", Trait: "app", DisplayName: "Payroll"},
		},
		Entitlements: []EntitlementData{
			{ResourceID: "payroll-app", EntitlementSlug: "admin", DisplayName: "Admin"},
		},
		ExternalGrants: grants,
	}
}

func TestValidateReferences_ExternalGrant_Valid(t *testing.T) {
	data := externalGrantBase(
		ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "all", MatchResourceType: "user"},
		ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "id", MatchResourceType: "group", MatchID: "grp-1"},
		ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "attribute", MatchResourceType: "user",
			MatchKey: "email", MatchValue: "jane@corp.com"},
		ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "id", MatchResourceType: "group", MatchID: "grp-2",
			ExpandEntitlementSlug: "member", ExpandDepth: "shallow"},
		ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "attribute", MatchResourceType: "group",
			MatchKey: "name", MatchValue: "Ext Group",
			ExpandEntitlementSlug: "member"},
	)
	if err := ValidateReferences(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateReferences_ExternalGrant_Errors(t *testing.T) {
	tests := []struct {
		name    string
		grant   ExternalGrant
		wantErr string
	}{
		{
			name: "missing resource_id",
			grant: ExternalGrant{EntitlementSlug: "admin",
				MatchType: "all", MatchResourceType: "user"},
			wantErr: "resource_id",
		},
		{
			name: "unknown resource_id",
			grant: ExternalGrant{ResourceID: "nope", EntitlementSlug: "admin",
				MatchType: "all", MatchResourceType: "user"},
			wantErr: "no user or resource with that ID",
		},
		{
			name: "unknown entitlement",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "nope",
				MatchType: "all", MatchResourceType: "user"},
			wantErr: "entitlement",
		},
		{
			name: "invalid match_resource_type",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "all", MatchResourceType: "role"},
			wantErr: "match_resource_type",
		},
		{
			name: "invalid match_type",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "fuzzy", MatchResourceType: "user"},
			wantErr: "match_type",
		},
		{
			name: "id without match_id",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "id", MatchResourceType: "user"},
			wantErr: "match_id",
		},
		{
			name: "attribute without match_value",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "attribute", MatchResourceType: "user", MatchKey: "email"},
			wantErr: "match_value",
		},
		{
			name: "expansion on user principal",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "id", MatchResourceType: "user", MatchID: "u1",
				ExpandEntitlementSlug: "member"},
			wantErr: "match_resource_type \"group\"",
		},
		{
			name: "expansion with match_type all",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "all", MatchResourceType: "group",
				ExpandEntitlementSlug: "member"},
			wantErr: "match_type \"id\" or \"attribute\"",
		},
		{
			name: "expand_depth without expand_entitlement_slug",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "id", MatchResourceType: "group", MatchID: "g1",
				ExpandDepth: "full"},
			wantErr: "expand_depth",
		},
		{
			name: "invalid expand_depth",
			grant: ExternalGrant{ResourceID: "payroll-app", EntitlementSlug: "admin",
				MatchType: "id", MatchResourceType: "group", MatchID: "g1",
				ExpandEntitlementSlug: "member", ExpandDepth: "deep"},
			wantErr: "expand_depth",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReferences(externalGrantBase(tc.grant))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected %q in error: %s", tc.wantErr, err.Error())
			}
		})
	}
}

func TestLoadYamlData_ExternalGrants(t *testing.T) {
	yamlContent := `
resources:
  - id: payroll-app
    resource_type: app
    trait: app
    display_name: Payroll
entitlements:
  - resource_id: payroll-app
    entitlement_slug: admin
external_grants:
  - resource_id: payroll-app
    entitlement_slug: admin
    match_type: attribute
    match_resource_type: user
    match_key: email
    match_value: jane@corp.com
`
	path := filepath.Join(t.TempDir(), "data.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := loadYamlData(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.ExternalGrants) != 1 {
		t.Fatalf("expected 1 external grant, got %d", len(data.ExternalGrants))
	}
	eg := data.ExternalGrants[0]
	if eg.ResourceID != "payroll-app" || eg.MatchType != "attribute" ||
		eg.MatchKey != "email" || eg.MatchValue != "jane@corp.com" {
		t.Errorf("unexpected external grant: %+v", eg)
	}
}

func TestLoadCSVData_ExternalGrants(t *testing.T) {
	csvContent := strings.Join([]string{
		"record_type,id,resource_type,trait,display_name,resource_id,entitlement_slug,match_type,match_resource_type,match_id,match_key,match_value",
		"resource,payroll-app,app,app,Payroll,,,,,,,",
		"entitlement,,,,,payroll-app,admin,,,,,",
		"external_grant,,,,,payroll-app,admin,id,group,grp-1,,",
		"external_grant,,,,,payroll-app,admin,attribute,user,,email,jane@corp.com",
	}, "\n")
	path := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(path, []byte(csvContent), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := loadCSVData(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.ExternalGrants) != 2 {
		t.Fatalf("expected 2 external grants, got %d", len(data.ExternalGrants))
	}
	if data.ExternalGrants[0].MatchType != "id" || data.ExternalGrants[0].MatchID != "grp-1" {
		t.Errorf("unexpected first external grant: %+v", data.ExternalGrants[0])
	}
	if data.ExternalGrants[1].MatchKey != "email" || data.ExternalGrants[1].MatchValue != "jane@corp.com" {
		t.Errorf("unexpected second external grant: %+v", data.ExternalGrants[1])
	}
}
