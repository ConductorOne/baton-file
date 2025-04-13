package connector

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

// fileSyncer implements the ResourceSyncer interface for a specific resource type.
// It holds a reference to the resource type it handles and the path to the data file.
// Data loading and caching is performed within each interface method call (List, Entitlements, Grants).
type fileSyncer struct {
	resourceType  *v2.ResourceType
	inputFilePath string
	// Caches are no longer stored here; they are built per-method call.
}

// newFileSyncer creates a new fileSyncer instance.
func newFileSyncer(ctx context.Context,
	rt *v2.ResourceType,
	filePath string,
) *fileSyncer {
	return &fileSyncer{
		resourceType:  rt,
		inputFilePath: filePath,
	}
}

// The ResourceType method returns the resource type definition handled by this syncer.
// It implements the ResourceType method, required by the connectorbuilder.ResourceSyncer interface.
// The connectorbuilder.ResourceSyncer interface uses this to associate the syncer with its definition.
// Which allows the SDK sync engine to know which resource type this syncer manages.
// The implementation directly returns the stored v2.ResourceType passed during initialization.
func (fs *fileSyncer) ResourceType(ctx context.Context) *v2.ResourceType {
	return fs.resourceType
}

// The List method retrieves a paginated list of resources for the syncer's type.
// It implements the List method, required by the connectorbuilder.ResourceSyncer interface.
// It loads data, builds resource/type caches, filters for the relevant type, and returns paginated results.
func (fs *fileSyncer) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	// 1. Load data for this specific call
	loadedData, err := LoadFileData(fs.inputFilePath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("List: failed to load data file: %w", err)
	}

	// 2. Build necessary caches locally for this call
	resourceTypesCache, err := buildResourceTypeCache(ctx, loadedData.Resources, loadedData.Users)
	if err != nil {
		return nil, "", nil, fmt.Errorf("List: failed to build resource type cache: %w", err)
	}
	resourceCache, err := buildResourceCache(ctx, loadedData.Users, loadedData.Resources, resourceTypesCache)
	if err != nil {
		return nil, "", nil, fmt.Errorf("List: failed to build resource cache: %w", err)
	}

	// 3. Filter resources from the locally built cache
	matchingResources := make([]*v2.Resource, 0)
	for _, res := range resourceCache {
		if res.Id.ResourceType != fs.resourceType.Id {
			continue
		}
		// Apply parent filtering logic
		if parentResourceID != nil {
			if res.ParentResourceId == nil || res.ParentResourceId.ResourceType != parentResourceID.ResourceType || res.ParentResourceId.Resource != parentResourceID.Resource {
				continue
			}
		} else {
			if res.ParentResourceId != nil {
				continue
			}
		}
		matchingResources = append(matchingResources, res)
	}

	// 4. Sort and Paginate
	sort.SliceStable(matchingResources, func(i, j int) bool {
		return matchingResources[i].Id.Resource < matchingResources[j].Id.Resource
	})

	pageSize := 50 // Consider making page size configurable?
	bag := &pagination.Bag{}
	err = bag.Unmarshal(pToken.Token)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to unmarshal pagination token: %w", err)
	}

	pageToken := bag.PageToken()
	pageOffset := 0
	if pageToken != "" {
		pageOffset, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to parse page token offset: %w", err)
		}
	}

	start := pageOffset
	end := start + pageSize
	if start >= len(matchingResources) {
		return nil, "", nil, nil
	}
	if end > len(matchingResources) {
		end = len(matchingResources)
	}

	rv := matchingResources[start:end]

	nextPageToken := ""
	if end < len(matchingResources) {
		nextPageToken, err = bag.NextToken(strconv.Itoa(end))
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to marshal next page token: %w", err)
		}
	}

	return rv, nextPageToken, nil, nil
}

// The Entitlements method retrieves a paginated list of entitlements for the syncer's type.
// It implements the Entitlements method, required by the connectorbuilder.ResourceSyncer interface.
// It loads data, builds resource/entitlement caches, filters for the relevant resource, and returns paginated results.
func (fs *fileSyncer) Entitlements(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	// 1. Load data for this specific call
	loadedData, err := LoadFileData(fs.inputFilePath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Entitlements: failed to load data file: %w", err)
	}

	// 2. Build necessary caches locally for this call
	resourceTypesCache, err := buildResourceTypeCache(ctx, loadedData.Resources, loadedData.Users)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Entitlements: failed to build resource type cache: %w", err)
	}
	resourceCache, err := buildResourceCache(ctx, loadedData.Users, loadedData.Resources, resourceTypesCache)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Entitlements: failed to build resource cache: %w", err)
	}
	entitlementCache, err := buildEntitlementCache(ctx, loadedData.Entitlements, resourceCache)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Entitlements: failed to build entitlement cache: %w", err)
	}

	// 3. Filter entitlements from the locally built cache
	matchingEntitlements := make([]*v2.Entitlement, 0)
	for _, ent := range entitlementCache {
		if ent.Resource.Id.ResourceType == resource.Id.ResourceType && ent.Resource.Id.Resource == resource.Id.Resource {
			matchingEntitlements = append(matchingEntitlements, ent)
		}
	}

	// 4. Sort and Paginate
	sort.SliceStable(matchingEntitlements, func(i, j int) bool {
		return matchingEntitlements[i].Slug < matchingEntitlements[j].Slug
	})

	pageSize := 50
	bag := &pagination.Bag{}
	err = bag.Unmarshal(pToken.Token)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to unmarshal pagination token: %w", err)
	}

	pageToken := bag.PageToken()
	pageOffset := 0
	if pageToken != "" {
		pageOffset, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to parse page token offset: %w", err)
		}
	}

	start := pageOffset
	end := start + pageSize
	if start >= len(matchingEntitlements) {
		return nil, "", nil, nil
	}
	if end > len(matchingEntitlements) {
		end = len(matchingEntitlements)
	}

	rv := matchingEntitlements[start:end]

	nextPageToken := ""
	if end < len(matchingEntitlements) {
		nextPageToken, err = bag.NextToken(strconv.Itoa(end))
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to marshal next page token: %w", err)
		}
	}

	return rv, nextPageToken, nil, nil
}

// The Grants method retrieves a paginated list of grants for the syncer's type.
// It implements the Grants method, required by the connectorbuilder.ResourceSyncer interface.
// It loads data, builds all caches, filters grants based on the resource context, and returns paginated results.
func (fs *fileSyncer) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	// 1. Load data for this specific call
	loadedData, err := LoadFileData(fs.inputFilePath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Grants: failed to load data file: %w", err)
	}

	// 2. Build necessary caches locally for this call
	resourceTypesCache, err := buildResourceTypeCache(ctx, loadedData.Resources, loadedData.Users)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Grants: failed to build resource type cache: %w", err)
	}
	resourceCache, err := buildResourceCache(ctx, loadedData.Users, loadedData.Resources, resourceTypesCache)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Grants: failed to build resource cache: %w", err)
	}
	entitlementCache, err := buildEntitlementCache(ctx, loadedData.Entitlements, resourceCache)
	if err != nil {
		return nil, "", nil, fmt.Errorf("Grants: failed to build entitlement cache: %w", err)
	}

	// 3. Filter grants based on the locally loaded grantData
	matchingGrants := make([]*v2.Grant, 0)

	for i, grantInfo := range loadedData.Grants {
		principalIdentifier := grantInfo.Principal
		entitlementIdentifier := grantInfo.EntitlementId

		// Find the principal resource object
		var principalResource *v2.Resource
		if res, ok := resourceCache[principalIdentifier]; ok {
			principalResource = res
		} else {
			if ent, ok := entitlementCache[principalIdentifier]; ok {
				principalResource = ent.Resource
			} else {
				l.Warn("Skipping grant: principal resource not found", zap.String("principal_identifier", principalIdentifier), zap.Int("grant_data_index", i))
				continue
			}
		}
		principalIdProto := principalResource.Id

		// Find target entitlement object from local cache
		targetEntitlement, ok := entitlementCache[entitlementIdentifier]
		if !ok {
			l.Warn("Skipping grant because target entitlement not found in local cache",
				zap.String("entitlement_id", entitlementIdentifier),
				zap.Int("grant_data_index", i),
			)
			continue
		}

		// Check if this grant corresponds to the resource handled by this syncer's Grants call
		grantMatchesContext := (principalIdProto.ResourceType == resource.Id.ResourceType && principalIdProto.Resource == resource.Id.Resource) ||
			(targetEntitlement.Resource.Id.ResourceType == resource.Id.ResourceType && targetEntitlement.Resource.Id.Resource == resource.Id.Resource)

		if !grantMatchesContext {
			continue
		}

		// Expansion Logic
		grantOptions := []grant.GrantOption{}
		principalResourceType, rtOk := resourceTypesCache[principalIdProto.ResourceType]
		if rtOk {
			isUserOrApp := resourceTypeHasTrait(principalResourceType, v2.ResourceType_TRAIT_USER) || resourceTypeHasTrait(principalResourceType, v2.ResourceType_TRAIT_APP)
			_, principalWasEntitlementKey := entitlementCache[principalIdentifier]

			if !isUserOrApp && principalWasEntitlementKey {
				membershipEntitlement := entitlementCache[principalIdentifier]
				expandableProto := &v2.GrantExpandable{EntitlementIds: []string{membershipEntitlement.Id}}
				grantOptions = append(grantOptions, grant.WithAnnotation(expandableProto))
			}
		} else {
			l.Warn("Could not find resource type for principal in local cache, skipping expansion check", zap.String("principal_type", principalIdProto.ResourceType), zap.Int("grant_data_index", i))
		}

		newGrant := grant.NewGrant(targetEntitlement.Resource, targetEntitlement.Slug, principalIdProto, grantOptions...)
		matchingGrants = append(matchingGrants, newGrant)
	}

	// 4. Sort and Paginate
	sort.SliceStable(matchingGrants, func(i, j int) bool {
		// Sort grants consistently, e.g., by principal ID then entitlement ID
		if matchingGrants[i].Principal.Id.String() != matchingGrants[j].Principal.Id.String() {
			return matchingGrants[i].Principal.Id.String() < matchingGrants[j].Principal.Id.String()
		}
		return matchingGrants[i].Entitlement.Id < matchingGrants[j].Entitlement.Id
	})

	pageSize := 50
	bag := &pagination.Bag{}
	err = bag.Unmarshal(pToken.Token)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to unmarshal pagination token: %w", err)
	}

	pageToken := bag.PageToken()
	pageOffset := 0
	if pageToken != "" {
		pageOffset, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to parse page token offset: %w", err)
		}
	}

	start := pageOffset
	end := start + pageSize
	if start >= len(matchingGrants) {
		return nil, "", nil, nil
	}
	if end > len(matchingGrants) {
		end = len(matchingGrants)
	}

	rv := matchingGrants[start:end]

	nextPageToken := ""
	if end < len(matchingGrants) {
		nextPageToken, err = bag.NextToken(strconv.Itoa(end))
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to marshal next page token: %w", err)
		}
	}

	return rv, nextPageToken, nil, nil
}

// resourceTypeHasTrait is a helper function to check if a resource type has a specific trait.
// It is used within the Grants method to determine if a principal is expandable.
func resourceTypeHasTrait(rt *v2.ResourceType, traitToFind v2.ResourceType_Trait) bool {
	if rt == nil {
		return false
	}
	for _, t := range rt.Traits {
		if t == traitToFind {
			return true
		}
	}
	return false
}
