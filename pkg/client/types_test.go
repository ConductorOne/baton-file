package client

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUserData_YAMLUnmarshal(t *testing.T) {
	input := `
id: jane.smith
display_name: Jane Smith
email: jane.smith@example.com
status: enabled
employee_id: "EMP-001"
login: jsmith
login_aliases:
  - jane.s
  - jsmith01
mfa_enabled: true
`
	var u UserData
	if err := yaml.Unmarshal([]byte(input), &u); err != nil {
		t.Fatal(err)
	}
	if u.ID != "jane.smith" {
		t.Errorf("expected id=jane.smith, got %s", u.ID)
	}
	if len(u.EmployeeID) != 1 || u.EmployeeID[0] != "EMP-001" {
		t.Errorf("expected employee_id=[EMP-001], got %v", u.EmployeeID)
	}
	if len(u.LoginAliases) != 2 {
		t.Errorf("expected 2 login_aliases, got %d", len(u.LoginAliases))
	}
	if u.MFAEnabled == nil || !*u.MFAEnabled {
		t.Error("mfa_enabled should be true")
	}
}

func TestUserData_JSONUnmarshal(t *testing.T) {
	input := `{
		"id": "john.doe",
		"display_name": "John Doe",
		"email": "john.doe@example.com",
		"employee_id": ["EMP-002", "C-99"],
		"mfa_enabled": false,
		"sso_enabled": true,
		"status_details": "On leave"
	}`
	var u UserData
	if err := json.Unmarshal([]byte(input), &u); err != nil {
		t.Fatal(err)
	}
	if u.ID != "john.doe" {
		t.Errorf("expected id=john.doe, got %s", u.ID)
	}
	if len(u.EmployeeID) != 2 {
		t.Errorf("expected 2 employee_ids, got %d", len(u.EmployeeID))
	}
	if u.MFAEnabled == nil || *u.MFAEnabled {
		t.Error("mfa should be false")
	}
	if u.SSOEnabled == nil || !*u.SSOEnabled {
		t.Error("sso should be true")
	}
	if u.StatusDetails != "On leave" {
		t.Errorf("expected status_details='On leave', got %s", u.StatusDetails)
	}
}

func TestResourceData_YAMLUnmarshal(t *testing.T) {
	input := `
resource_type: team
trait: group
id: engineering
display_name: Engineering
parent_resource: acme-org
profile:
  slack_channel: "#eng"
`
	var r ResourceData
	if err := yaml.Unmarshal([]byte(input), &r); err != nil {
		t.Fatal(err)
	}
	if r.Trait != "group" {
		t.Errorf("expected trait=group, got %s", r.Trait)
	}
	if r.ID != "engineering" {
		t.Errorf("expected id=engineering, got %s", r.ID)
	}
	if r.Profile["slack_channel"] != "#eng" {
		t.Errorf("expected profile.slack_channel=#eng, got %v", r.Profile["slack_channel"])
	}
}

func TestEntitlementData_YAMLUnmarshal(t *testing.T) {
	input := `
resource_id: engineering
entitlement_slug: member
display_name: Team Member
`
	var e EntitlementData
	if err := yaml.Unmarshal([]byte(input), &e); err != nil {
		t.Fatal(err)
	}
	if e.ResourceID != "engineering" {
		t.Errorf("expected resource_id=engineering, got %s", e.ResourceID)
	}
	if e.EntitlementSlug != "member" {
		t.Errorf("expected entitlement_slug=member, got %s", e.EntitlementSlug)
	}
}

func TestDirectUserGrant_YAMLUnmarshal(t *testing.T) {
	input := `
principal_id: jane.smith
resource_id: engineering
entitlement_slug: member
`
	var g DirectUserGrant
	if err := yaml.Unmarshal([]byte(input), &g); err != nil {
		t.Fatal(err)
	}
	if g.PrincipalID != "jane.smith" || g.ResourceID != "engineering" || g.EntitlementSlug != "member" {
		t.Errorf("unexpected: %+v", g)
	}
}

func TestGrantInheritanceMapping_YAMLUnmarshal(t *testing.T) {
	input := `
principal_resource_id: engineering
principal_entitlement_slug: member
inherited_resource_id: api-service
inherited_entitlement_slug: read
inheritance_depth: full
`
	var m GrantInheritanceMapping
	if err := yaml.Unmarshal([]byte(input), &m); err != nil {
		t.Fatal(err)
	}
	if m.InheritanceDepth != "full" {
		t.Errorf("expected depth=full, got %s", m.InheritanceDepth)
	}
}

func TestLoadedData_YAMLUnmarshal(t *testing.T) {
	input := `
users:
  - id: admin
    display_name: Admin
direct_user_grants:
  - principal_id: admin
    resource_id: team
    entitlement_slug: member
grant_inheritance_mappings: []
`
	var d LoadedData
	if err := yaml.Unmarshal([]byte(input), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Users) != 1 {
		t.Errorf("expected 1 user, got %d", len(d.Users))
	}
	if len(d.DirectUserGrants) != 1 {
		t.Errorf("expected 1 grant, got %d", len(d.DirectUserGrants))
	}
}
