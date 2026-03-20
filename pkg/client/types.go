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
}
