// Package models defines the primary data structures used within the file connector.

package connector

import (
	"context"
	"fmt"
)

// The FileConnector struct is the main implementation of the Baton connector for file processing.
// It is required by the connectorbuilder.Connector interface for defining connector behavior.
// It holds the path to the input data file.
// The structure provides the context (file path) needed for loading data during sync operations.
// Instances are created by NewFileConnector.
type FileConnector struct {
	inputFilePath string // Path to the input data file (.xlsx, .yaml, .json)
}

// LoadedData holds all the data parsed from the input file.
// It is the top-level structure used to unmarshal data from YAML/JSON files.
type LoadedData struct {
	Users        []UserData        `yaml:"users" json:"users"`
	Resources    []ResourceData    `yaml:"resources" json:"resources"`
	Entitlements []EntitlementData `yaml:"entitlements" json:"entitlements"`
	Grants       []GrantData       `yaml:"grants" json:"grants"`
}

// The UserData struct holds raw data corresponding to a row in the 'users' tab.
// It is defined for parsing data into an intermediary Go representation.
// It holds fields Name, DisplayName, Email, Status, Type, and a map for dynamic Profile attributes.
// The structure represents a single user definition before conversion to an SDK Resource object with a User trait.
type UserData struct {
	Name        string                 `yaml:"name" json:"name"`
	DisplayName string                 `yaml:"display_name" json:"display_name"`
	Email       string                 `yaml:"email" json:"email"`
	Status      string                 `yaml:"status" json:"status"`
	Type        string                 `yaml:"type" json:"type"`       // Expected: "human" or "service" (maps to UserTrait_AccountType)
	Profile     map[string]interface{} `yaml:"profile" json:"profile"` // For user_profile_* columns / profile map
}

// The ResourceData struct holds raw data corresponding to a row in the 'resources' tab.
// It is defined for parsing data into an intermediary Go representation.
// It holds fields ResourceType (e.g., "role"), ResourceFunction (trait string like "group"), Name, DisplayName, Description, ParentResource.
// The structure represents a single resource definition before conversion to an SDK Resource object.
type ResourceData struct {
	ResourceType     string `yaml:"resource_type" json:"resource_type"`         // Resource Type string (e.g., "role", "team", "workspace")
	ResourceFunction string `yaml:"resource_function" json:"resource_function"` // Resource Function string (trait name like "group", "role")
	Name             string `yaml:"name" json:"name"`                           // Unique name/ID of the resource
	DisplayName      string `yaml:"display_name" json:"display_name"`
	Description      string `yaml:"description" json:"description"`
	ParentResource   string `yaml:"parent_resource" json:"parent_resource"` // Name/ID of the parent resource, if any
}

// The EntitlementData struct holds raw data corresponding to a row in the 'entitlements' tab.
// It is defined for parsing data into an intermediary Go representation.
// It holds fields ResourceName (the resource it's defined on), Entitlement (acting as the slug), DisplayName, and Description.
// The structure represents a single entitlement definition before conversion to an SDK Entitlement object.
type EntitlementData struct {
	ResourceName string `yaml:"resource_name" json:"resource_name"` // Name/ID of the resource this entitlement is defined ON
	Entitlement  string `yaml:"entitlement" json:"entitlement"`     // The acts as the Slug
	DisplayName  string `yaml:"display_name" json:"display_name"`
	Description  string `yaml:"description" json:"description"`
}

// The GrantData struct holds raw data corresponding to a row in the 'grants' tab.
// It is defined for parsing data into an intermediary Go representation.
// It holds fields Principal (type:name[:membership_slug]) and EntitlementId (resource_name:entitlement_slug).
// The structure represents a single grant relationship before conversion to an SDK Grant object.
type GrantData struct {
	Principal     string `yaml:"principal" json:"principal"`           // Format: "name" or "entitlement_id"
	EntitlementId string `yaml:"entitlement_id" json:"entitlement_id"` // Format: "resource_name:entitlement_slug"
}

// NewFileConnector creates a new instance of the FileConnector.
// The function is the constructor used by the main command to initialize the connector.
// The main command requires this constructor to instantiate the connector server.
// Which provides the application entry point with a configured connector instance.
// The implementation stores the provided file path for use during syncs.
func NewFileConnector(ctx context.Context, filePath string) (*FileConnector, error) {
	// Basic validation - ensure file path is not empty
	if filePath == "" {
		return nil, fmt.Errorf("input file path cannot be empty")
	}

	// Could add more validation here if needed (e.g., check extension initially)

	return &FileConnector{
		inputFilePath: filePath,
	}, nil
}
