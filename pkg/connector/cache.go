package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// The buildResourceTypeCache function constructs a map of resource type definitions based on the 'Resources' sheet.
// It is used by the ResourceSyncers method for creating resource type definitions based on data provided in the file.
// The ResourceSyncers method requires these definitions to understand resource kinds and their associated traits.
func buildResourceTypeCache(ctx context.Context, resources []ResourceData, users []UserData) (map[string]*v2.ResourceType, error) {
	l := ctxzap.Extract(ctx)
	l.Debug("Building resource type cache from Resource Function column")

	resourceTypes := make(map[string]*v2.ResourceType)

	if len(users) > 0 {
		resourceTypes["user"] = &v2.ResourceType{
			Id:          "user",
			DisplayName: "User",
			Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
		}
	}

	for _, rData := range resources {
		typeStringLower := strings.ToLower(rData.ResourceType)
		if typeStringLower == "user" {
			continue
		}
		if _, exists := resourceTypes[typeStringLower]; exists {
			continue
		}

		var traits []v2.ResourceType_Trait
		traitStringLower := strings.ToLower(rData.ResourceFunction)
		if traitEnum, ok := TraitMap[traitStringLower]; ok {
			traits = append(traits, traitEnum)
		} else {
			l.Warn("Unrecognized Resource Function for resource type, defaulting to TRAIT_UNSPECIFIED",
				zap.String("resource_type", typeStringLower),
				zap.String("resource_function", rData.ResourceFunction),
			)
			traits = append(traits, v2.ResourceType_TRAIT_UNSPECIFIED) // Default to UNSPECIFIED if not mapped
		}

		displayName := cases.Title(language.English).String(typeStringLower)

		resourceTypes[typeStringLower] = &v2.ResourceType{
			Id:          typeStringLower,
			DisplayName: displayName,
			Traits:      traits,
		}
		l.Debug("Defined resource type", zap.String("id", typeStringLower), zap.String("trait", traits[0].String()))
	}

	if len(resourceTypes) == 0 && len(users) == 0 {
		return nil, fmt.Errorf("no resource types could be found in resource data, and no users found")
	}

	l.Info("Built resource type cache from resource data", zap.Int("count", len(resourceTypes)))
	return resourceTypes, nil
}

// TraitMap maps lowercase string representations of traits to the corresponding SDK enum.
var TraitMap = map[string]v2.ResourceType_Trait{
	"user":   v2.ResourceType_TRAIT_USER,
	"group":  v2.ResourceType_TRAIT_GROUP,
	"role":   v2.ResourceType_TRAIT_ROLE,
	"app":    v2.ResourceType_TRAIT_APP,
	"secret": v2.ResourceType_TRAIT_SECRET,
}

// The buildResourceCache function constructs a map of resource objects from the loaded data.
// It is called by syncer methods to create resource instances based on UserData and ResourceData.
// The SDK requires these v2.Resource objects, including trait annotations, for various operations like listing and grant processing.
// The implementation processes users (including parsing LastLogin string in MM/DD/YYYY format) and other resources,
// uses rs.NewUserResource or rs.NewResource with appropriate rs.WithXxxTrait options, and returns the cache map keyed by resource name/ID.
func buildResourceCache(ctx context.Context, users []UserData, resources []ResourceData, resourceTypes map[string]*v2.ResourceType) (map[string]*v2.Resource, error) {
	l := ctxzap.Extract(ctx)
	cache := make(map[string]*v2.Resource)

	userResourceType, userTypeFound := resourceTypes["user"]
	if !userTypeFound && len(users) > 0 {
		return nil, fmt.Errorf("'user' resource type is not defined but user data exists")
	}

	for i, userData := range users {
		if userData.Name == "" {
			l.Warn("Skipping user entry with empty name", zap.Int("row_index", i+2))
			continue
		}
		if _, exists := cache[userData.Name]; exists {
			l.Error("Duplicate resource ID found (user defined multiple times or conflicts with non-user resource)",
				zap.String("resource_id", userData.Name),
				zap.Int("user_row_index", i+2),
			)
			continue
		}

		var userOpts []rs.UserTraitOption
		if userData.Email != "" {
			userOpts = append(userOpts, rs.WithEmail(userData.Email, true))
		}
		if len(userData.Profile) > 0 {
			userOpts = append(userOpts, rs.WithUserProfile(userData.Profile))
		}

		userStatus := v2.UserTrait_Status_STATUS_ENABLED
		statusLower := strings.ToLower(strings.TrimSpace(userData.Status))
		if statusLower != "" {
			switch statusLower {
			case "enabled", "active":
				userStatus = v2.UserTrait_Status_STATUS_ENABLED
			case "disabled", "inactive", "suspended":
				userStatus = v2.UserTrait_Status_STATUS_DISABLED
			default:
				l.Warn("Unrecognized user status, defaulting to ENABLED",
					zap.String("user_name", userData.Name),
					zap.String("status_value", userData.Status),
					zap.Int("row_index", i+2),
				)
			}
		}
		userOpts = append(userOpts, rs.WithStatus(userStatus))

		userAccountType := v2.UserTrait_ACCOUNT_TYPE_HUMAN
		accountTypeLower := strings.ToLower(strings.TrimSpace(userData.Type))
		if accountTypeLower != "" {
			switch accountTypeLower {
			case "service", "system", "bot", "machine":
				userAccountType = v2.UserTrait_ACCOUNT_TYPE_SERVICE
			case "human", "user", "person":
				userAccountType = v2.UserTrait_ACCOUNT_TYPE_HUMAN
			default:
				l.Warn("Unrecognized account_type, defaulting to HUMAN",
					zap.String("user_name", userData.Name),
					zap.String("account_type_value", userData.Type),
					zap.Int("row_index", i+2),
				)
			}
		}
		userOpts = append(userOpts, rs.WithAccountType(userAccountType))

		if userData.LastLogin != "" {
			lastLoginTime, err := time.Parse("01/02/2006", userData.LastLogin)
			if err != nil {
				l.Warn("Failed to parse LastLogin date for user, skipping field (expected format MM/DD/YYYY)",
					zap.String("user_name", userData.Name),
					zap.String("last_login_value", userData.LastLogin),
					zap.Error(err),
					zap.Int("row_index", i+2),
				)
			} else {
				userOpts = append(userOpts, rs.WithLastLogin(lastLoginTime))
			}
		}

		userResource, err := rs.NewUserResource(userData.DisplayName, userResourceType, userData.Name, userOpts)
		if err != nil {
			l.Error("Failed to create user resource object", zap.Error(err), zap.String("user_name", userData.Name))
			continue
		}
		cache[userData.Name] = userResource
	}

	for i, resourceData := range resources {
		if resourceData.Name == "" {
			l.Warn("Skipping resource entry with empty name", zap.Int("row_index", i+2))
			continue
		}
		if _, exists := cache[resourceData.Name]; exists {
			l.Error("Duplicate resource ID found (resource defined multiple times or conflicts with user)",
				zap.String("resource_id", resourceData.Name),
				zap.Int("resource_row_index", i+2),
			)
			continue
		}

		resourceType, typeExists := resourceTypes[strings.ToLower(resourceData.ResourceType)]
		if !typeExists {
			l.Error("Resource type specified for resource not found in resource_types data",
				zap.String("resource_name", resourceData.Name),
				zap.String("resource_type", resourceData.ResourceType),
				zap.Int("row_index", i+2),
			)
			continue
		}

		var resourceOptions []rs.ResourceOption
		if len(resourceType.Traits) > 0 {
			switch resourceType.Traits[0] {
			case v2.ResourceType_TRAIT_USER:
				resourceOptions = append(resourceOptions, rs.WithUserTrait())
			case v2.ResourceType_TRAIT_GROUP:
				resourceOptions = append(resourceOptions, rs.WithGroupTrait())
			case v2.ResourceType_TRAIT_ROLE:
				resourceOptions = append(resourceOptions, rs.WithRoleTrait())
			case v2.ResourceType_TRAIT_APP:
				resourceOptions = append(resourceOptions, rs.WithAppTrait())
			case v2.ResourceType_TRAIT_SECRET:
				resourceOptions = append(resourceOptions, rs.WithSecretTrait())
			}
		}

		res, err := rs.NewResource(
			resourceData.DisplayName,
			resourceType,
			resourceData.Name,
			resourceOptions...,
		)
		if err != nil {
			l.Error("Failed to create resource object", zap.Error(err), zap.String("resource_name", resourceData.Name))
			continue
		}

		cache[resourceData.Name] = res
	}

	for _, resourceData := range resources {
		if resourceData.ParentResource == "" {
			continue
		}

		resource, exists := cache[resourceData.Name]
		if !exists {
			continue
		}

		parentResource, parentExists := cache[resourceData.ParentResource]
		if !parentExists {
			l.Error("Parent resource not found for child resource",
				zap.String("child_resource", resourceData.Name),
				zap.String("parent_resource", resourceData.ParentResource))
			continue
		}

		parentID := &v2.ResourceId{
			ResourceType: parentResource.Id.ResourceType,
			Resource:     parentResource.Id.Resource,
		}

		err := rs.WithParentResourceID(parentID)(resource)
		if err != nil {
			l.Error("Failed to set parent resource ID",
				zap.String("child_resource", resourceData.Name),
				zap.String("parent_resource", resourceData.ParentResource),
				zap.Error(err))
		}
	}

	l.Info("Built resource cache", zap.Int("count", len(cache)))
	return cache, nil
}

// The buildEntitlementCache function constructs a map of entitlement definitions from the loaded data.
// It is called by syncer methods to create entitlement definitions based on EntitlementData.
// The SDK requires these v2.Entitlement objects for grant processing and representing permissions.
// The implementation iterates EntitlementData, creates v2.Entitlement objects using SDK helpers, links to parent resources, and returns cache map keyed by composite ID.
func buildEntitlementCache(ctx context.Context, entitlements []EntitlementData, resourceCache map[string]*v2.Resource) (map[string]*v2.Entitlement, error) {
	l := ctxzap.Extract(ctx)
	cache := make(map[string]*v2.Entitlement)

	for i, data := range entitlements {
		resourceName := data.ResourceName
		slug := data.Entitlement // The 'entitlement' column now acts as the slug

		if resourceName == "" {
			l.Warn("Skipping entitlement entry with empty resource_name", zap.Int("row_index", i+2))
			continue
		}
		if slug == "" {
			l.Warn("Skipping entitlement entry with empty entitlement (slug)", zap.String("resource_name", resourceName), zap.Int("row_index", i+2))
			continue
		}

		cacheKey := fmt.Sprintf("%s:%s", resourceName, slug)

		if _, exists := cache[cacheKey]; exists {
			l.Error("Duplicate entitlement key found (resource_name:entitlement)",
				zap.String("entitlement_key", cacheKey),
				zap.Int("row_index", i+2),
			)
			continue
		}

		parentResource, ok := resourceCache[resourceName]
		if !ok {
			l.Error("Parent resource for entitlement not found in resource cache",
				zap.String("entitlement_key", cacheKey),
				zap.String("resource_name", resourceName),
				zap.Int("row_index", i+2),
			)
			continue
		}

		entitlementOptions := []entitlement.EntitlementOption{
			entitlement.WithDisplayName(data.DisplayName),
			entitlement.WithDescription(data.Description),
		}

		ent := entitlement.NewAssignmentEntitlement(parentResource, slug, entitlementOptions...)

		cache[cacheKey] = ent
	}

	l.Info("Built entitlement cache", zap.Int("count", len(cache)))
	return cache, nil
}
