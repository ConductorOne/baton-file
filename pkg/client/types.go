package client

type UserData struct {
	ID               string                 `yaml:"id" json:"id"`
	DisplayName      string                 `yaml:"display_name" json:"display_name"`
	Email            string                 `yaml:"email" json:"email"`
	Status           string                 `yaml:"status" json:"status"`
	LastLogin        string                 `yaml:"last_login" json:"last_login"`
	Type             string                 `yaml:"type" json:"type"`
	Profile          map[string]interface{} `yaml:"profile" json:"profile"`
	EmployeeID       FlexibleStringList     `yaml:"employee_id" json:"employee_id"`
	Login            string                 `yaml:"login" json:"login"`
	LoginAliases     FlexibleStringList     `yaml:"login_aliases" json:"login_aliases"`
	AdditionalEmails FlexibleStringList     `yaml:"additional_emails" json:"additional_emails"`
	MFAEnabled       *bool                  `yaml:"mfa_enabled" json:"mfa_enabled"`
	SSOEnabled       *bool                  `yaml:"sso_enabled" json:"sso_enabled"`
	StatusDetails    string                 `yaml:"status_details" json:"status_details"`
}

type ResourceData struct {
	ResourceType   string                 `yaml:"resource_type" json:"resource_type"`
	Trait          string                 `yaml:"trait" json:"trait"`
	ID             string                 `yaml:"id" json:"id"`
	DisplayName    string                 `yaml:"display_name" json:"display_name"`
	Description    string                 `yaml:"description" json:"description"`
	ParentResource string                 `yaml:"parent_resource" json:"parent_resource"`
	Profile        map[string]interface{} `yaml:"profile" json:"profile"`
	CreatedAt      string                 `yaml:"created_at" json:"created_at"`
	ExpiresAt      string                 `yaml:"expires_at" json:"expires_at"`
	CreatedBy      string                 `yaml:"created_by" json:"created_by"`
	Identity       string                 `yaml:"identity" json:"identity"`
}

type EntitlementData struct {
	ResourceID      string `yaml:"resource_id" json:"resource_id"`
	EntitlementSlug string `yaml:"entitlement_slug" json:"entitlement_slug"`
	DisplayName     string `yaml:"display_name" json:"display_name"`
	Description     string `yaml:"description" json:"description"`
}

type DirectUserGrant struct {
	PrincipalID     string `yaml:"principal_id" json:"principal_id"`
	ResourceID      string `yaml:"resource_id" json:"resource_id"`
	EntitlementSlug string `yaml:"entitlement_slug" json:"entitlement_slug"`
}

// ExternalGrant assigns a local entitlement to a principal owned by another
// connector (the "shared identity source"). The principal is not defined in
// this file; instead a match rule describes how the SDK finds it in the
// external connector's synced data.
type ExternalGrant struct {
	ResourceID      string `yaml:"resource_id" json:"resource_id"`
	EntitlementSlug string `yaml:"entitlement_slug" json:"entitlement_slug"`
	// MatchType is one of "all", "id", or "attribute".
	MatchType string `yaml:"match_type" json:"match_type"`
	// MatchResourceType is the external principal's trait: "user" or "group".
	MatchResourceType string `yaml:"match_resource_type" json:"match_resource_type"`
	// MatchID is required when MatchType is "id" — the external principal's native ID.
	MatchID string `yaml:"match_id" json:"match_id"`
	// MatchKey/MatchValue are required when MatchType is "attribute",
	// e.g. key "email", value "jane@corp.com".
	MatchKey   string `yaml:"match_key" json:"match_key"`
	MatchValue string `yaml:"match_value" json:"match_value"`
	// ExpandEntitlementSlug optionally enables grant expansion: members of the
	// matched external group inherit this grant through the group's named
	// entitlement (e.g. "member"). Only valid when MatchResourceType is
	// "group" and MatchType is "id" or "attribute".
	ExpandEntitlementSlug string `yaml:"expand_entitlement_slug" json:"expand_entitlement_slug"`
	// ExpandDepth is "full" (transitive, default) or "shallow" (one level).
	// Only meaningful when ExpandEntitlementSlug is set.
	ExpandDepth string `yaml:"expand_depth" json:"expand_depth"`
}

type GrantInheritanceMapping struct {
	PrincipalResourceID      string `yaml:"principal_resource_id" json:"principal_resource_id"`
	PrincipalEntitlementSlug string `yaml:"principal_entitlement_slug" json:"principal_entitlement_slug"`
	InheritedResourceID      string `yaml:"inherited_resource_id" json:"inherited_resource_id"`
	InheritedEntitlementSlug string `yaml:"inherited_entitlement_slug" json:"inherited_entitlement_slug"`
	InheritanceDepth         string `yaml:"inheritance_depth" json:"inheritance_depth"`
}

type LoadedData struct {
	Users                    []UserData                `yaml:"users" json:"users"`
	Resources                []ResourceData            `yaml:"resources" json:"resources"`
	Entitlements             []EntitlementData         `yaml:"entitlements" json:"entitlements"`
	DirectUserGrants         []DirectUserGrant         `yaml:"direct_user_grants" json:"direct_user_grants"`
	GrantInheritanceMappings []GrantInheritanceMapping `yaml:"grant_inheritance_mappings" json:"grant_inheritance_mappings"`
	ExternalGrants           []ExternalGrant           `yaml:"external_grants" json:"external_grants"`
}
