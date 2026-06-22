package connector

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/conductorone/baton-file/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type FileConnector struct {
	inputFilePath string
	validatedData *client.LoadedData
}

func (fc *FileConnector) Close() error { return nil }

type syncCache struct {
	resourceTypes map[string]*v2.ResourceType
	resources     map[string]*v2.Resource
	entitlements  map[string]*v2.Entitlement

	// Pre-built indexes: constructed once in newSyncCache(), sorted, and reused
	// on every SDK call. Values are pointers into resources/entitlements — no
	// extra memory beyond the slice headers themselves.
	listIndex   map[string][]*v2.Resource    // key: listKey(typeID, parent)
	entIndex    map[string][]*v2.Entitlement // key: resourceID
	grantsIndex map[string][]*v2.Grant       // key: resourceID
}

// listKey returns the lookup key for the listIndex.
// top-level resources (no parent) use just the typeID.
func listKey(typeID string, parent *v2.ResourceId) string {
	if parent == nil {
		return typeID
	}
	return typeID + "/" + parent.GetResourceType() + "/" + parent.GetResource()
}

func New(ctx context.Context, v *viper.Viper, opts *cli.ConnectorOpts) (connectorbuilder.ConnectorBuilderV2, []connectorbuilder.Opt, error) {
	inputFile := v.GetString("input")
	if inputFile == "" {
		return nil, nil, fmt.Errorf("baton-file: --input file path is required")
	}
	return &FileConnector{inputFilePath: inputFile}, nil, nil
}

func (fc *FileConnector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "File Connector",
		Description: "Connector that syncs identity and access data from a local file",
	}, nil
}

func (fc *FileConnector) Validate(ctx context.Context) (annotations.Annotations, error) {
	_, err := os.Stat(fc.inputFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("baton-file: input file not found: %s", fc.inputFilePath)
		}
		return nil, fmt.Errorf("baton-file: error accessing input file: %w", err)
	}

	data, err := client.LoadFileData(fc.inputFilePath)
	if err != nil {
		return nil, fmt.Errorf("baton-file: input file is invalid: %w", err)
	}

	if err := client.ValidateUniqueIDs(data); err != nil {
		return nil, err
	}

	if err := client.ValidateTraits(data); err != nil {
		return nil, err
	}

	if err := client.ValidateEntitlementFields(data); err != nil {
		return nil, err
	}

	if err := client.ValidateSecretFields(data); err != nil {
		return nil, err
	}

	if err := client.ValidateReferences(data); err != nil {
		return nil, err
	}

	fc.validatedData = data

	return nil, nil
}

func (fc *FileConnector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncerV2 {
	l := ctxzap.Extract(ctx)

	// Validate() caches parsed data; reuse it here to avoid a double load.
	// Fallback re-loads if Validate() wasn't called. This method's signature
	// cannot return an error, so log-and-return-nil is intentional — the
	// connector stays alive for the next sync cycle.
	loadedData := fc.validatedData
	if loadedData == nil {
		var err error
		loadedData, err = client.LoadFileData(fc.inputFilePath)
		if err != nil {
			l.Error("baton-file: failed to load input file", zap.Error(err))
			return nil
		}
		if err := client.ValidateUniqueIDs(loadedData); err != nil {
			l.Error("baton-file: validation failed", zap.Error(err))
			return nil
		}
		if err := client.ValidateTraits(loadedData); err != nil {
			l.Error("baton-file: validation failed", zap.Error(err))
			return nil
		}
		if err := client.ValidateEntitlementFields(loadedData); err != nil {
			l.Error("baton-file: validation failed", zap.Error(err))
			return nil
		}
		if err := client.ValidateSecretFields(loadedData); err != nil {
			l.Error("baton-file: validation failed", zap.Error(err))
			return nil
		}
		if err := client.ValidateReferences(loadedData); err != nil {
			l.Error("baton-file: validation failed", zap.Error(err))
			return nil
		}
	}
	fc.validatedData = nil

	cache, err := newSyncCache(ctx, loadedData)
	if err != nil {
		l.Error("baton-file: failed to build sync cache", zap.Error(err))
		return nil
	}

	var syncers []connectorbuilder.ResourceSyncerV2
	for _, rt := range cache.resourceTypes {
		syncers = append(syncers, &resourceBuilder{cache: cache, resourceType: rt})
	}

	l.Info("baton-file: created resource syncers", zap.Int("count", len(syncers)))
	return syncers
}

func newSyncCache(ctx context.Context, data *client.LoadedData) (*syncCache, error) {
	l := ctxzap.Extract(ctx)
	c := &syncCache{}

	// 1. Discover resource types
	c.resourceTypes = make(map[string]*v2.ResourceType)
	if len(data.Users) > 0 {
		c.resourceTypes[userResourceType.GetId()] = userResourceType
	}
	for _, r := range data.Resources {
		typeID := strings.ToLower(r.ResourceType)
		if typeID == userResourceType.GetId() {
			continue
		}
		if existing, exists := c.resourceTypes[typeID]; exists {
			existingTrait := ""
			if len(existing.GetTraits()) > 0 {
				existingTrait = existing.GetTraits()[0].String()
			}
			newTrait := strings.ToLower(r.Trait)
			if TraitMap[newTrait].String() != existingTrait {
				l.Warn("baton-file: resource type has conflicting traits, using first occurrence",
					zap.String("resource_type", r.ResourceType),
					zap.String("resource_id", r.ID),
					zap.String("expected_trait", existingTrait),
					zap.String("conflicting_trait", r.Trait))
			}
			continue
		}
		c.resourceTypes[typeID] = buildDynamicResourceType(r.ResourceType, r.Trait)
	}

	// 2. Build user resources.
	// IDs are globally unique across users and resources by design — grants and
	// parent references use raw IDs, so ambiguity would break lookups downstream.
	// Documented in the XLSX Instructions sheet (rule #1) and all format docs.
	c.resources = make(map[string]*v2.Resource)
	var buildErrors []string
	for _, u := range data.Users {
		if u.ID == "" {
			l.Warn("baton-file: skipping user with empty id")
			continue
		}
		if _, exists := c.resources[u.ID]; exists {
			l.Warn("baton-file: duplicate resource id, skipping", zap.String("id", u.ID))
			continue
		}
		res, err := buildUserResource(ctx, u, userResourceType)
		if err != nil {
			buildErrors = append(buildErrors, fmt.Sprintf("user %s: %v", u.ID, err))
			continue
		}
		c.resources[u.ID] = res
	}

	// 3. Build non-user resources
	for _, r := range data.Resources {
		if r.ID == "" {
			l.Warn("baton-file: skipping resource with empty id")
			continue
		}
		if existing, exists := c.resources[r.ID]; exists {
			l.Warn("baton-file: resource id conflicts with an existing resource or user, skipping",
				zap.String("id", r.ID),
				zap.String("existing_type", existing.GetId().GetResourceType()))
			continue
		}
		rt := c.resourceTypes[strings.ToLower(r.ResourceType)]
		if rt == nil {
			buildErrors = append(buildErrors, fmt.Sprintf("resource %s: unknown resource type %q", r.ID, r.ResourceType))
			continue
		}
		res, err := buildResource(ctx, r, rt)
		if err != nil {
			buildErrors = append(buildErrors, fmt.Sprintf("resource %s: %v", r.ID, err))
			continue
		}
		c.resources[r.ID] = res
	}

	if len(buildErrors) > 0 {
		return nil, fmt.Errorf("baton-file: failed to build %d resource(s): %s",
			len(buildErrors), strings.Join(buildErrors, "; "))
	}

	// 4. Wire parent resources
	for _, r := range data.Resources {
		if r.ParentResource == "" {
			continue
		}
		child, parent := c.resources[r.ID], c.resources[r.ParentResource]
		if child == nil || parent == nil {
			l.Warn("baton-file: skipping parent wiring, missing child or parent",
				zap.String("child_id", r.ID), zap.String("parent_id", r.ParentResource))
			continue
		}
		// On failure the resource stays in the cache without its parent.
		// Removing it would break any grants referencing it — keeping a
		// parentless resource is the lesser problem.
		if err := rs.WithParentResourceID(parent.GetId())(child); err != nil {
			l.Warn("baton-file: failed to set parent resource id",
				zap.String("child_id", r.ID), zap.String("parent_id", r.ParentResource), zap.Error(err))
		}
	}

	// 5. Add ChildResourceType annotations
	childTypesByParent := make(map[string]map[string]struct{})
	for _, r := range data.Resources {
		if r.ParentResource == "" {
			continue
		}
		if _, ok := childTypesByParent[r.ParentResource]; !ok {
			childTypesByParent[r.ParentResource] = make(map[string]struct{})
		}
		childTypesByParent[r.ParentResource][strings.ToLower(r.ResourceType)] = struct{}{}
	}
	for parentID, childTypes := range childTypesByParent {
		parent, ok := c.resources[parentID]
		if !ok {
			l.Warn("baton-file: parent for ChildResourceType annotation not found",
				zap.String("parent_id", parentID))
			continue
		}
		annos := annotations.Annotations(parent.GetAnnotations())
		for childTypeID := range childTypes {
			annos.Append(
				v2.ChildResourceType_builder{ResourceTypeId: childTypeID}.Build(),
			)
		}
		parent.Annotations = annos
	}

	// 6. Build entitlements
	c.entitlements = make(map[string]*v2.Entitlement)
	for _, e := range data.Entitlements {
		if e.ResourceID == "" || e.EntitlementSlug == "" {
			l.Warn("baton-file: skipping entitlement with empty resource_id or slug")
			continue
		}
		parentRes := c.resources[e.ResourceID]
		if parentRes == nil {
			l.Warn("baton-file: parent resource for entitlement not found", zap.String("resource_id", e.ResourceID))
			continue
		}
		ent := entitlement.NewAssignmentEntitlement(parentRes, e.EntitlementSlug,
			entitlement.WithDisplayName(e.DisplayName),
			entitlement.WithDescription(e.Description),
		)
		key := fmt.Sprintf("%s:%s", e.ResourceID, e.EntitlementSlug)
		c.entitlements[key] = ent
	}

	// 7. Pre-build grants index.
	// All grants are constructed and sorted once here, indexed by resource ID.
	// Grants() becomes an O(1) lookup + slice — no per-page scan or sort.
	c.grantsIndex = make(map[string][]*v2.Grant)

	for i, g := range data.DirectUserGrants {
		res, ok := c.resources[g.ResourceID]
		if !ok {
			l.Warn("baton-file: skipping direct grant, resource not found",
				zap.String("resource_id", g.ResourceID), zap.Int("index", i))
			continue
		}
		principalRes, ok := c.resources[g.PrincipalID]
		if !ok {
			l.Warn("baton-file: skipping direct grant, principal id not found",
				zap.String("principal_id", g.PrincipalID), zap.Int("index", i))
			continue
		}
		if principalRes.GetId().GetResourceType() != userResourceType.GetId() {
			l.Warn("baton-file: skipping direct grant, principal is not a user",
				zap.String("principal_id", g.PrincipalID),
				zap.String("actual_type", principalRes.GetId().GetResourceType()),
				zap.Int("index", i))
			continue
		}
		entKey := fmt.Sprintf("%s:%s", g.ResourceID, g.EntitlementSlug)
		if _, ok := c.entitlements[entKey]; !ok {
			l.Warn("baton-file: skipping direct grant, entitlement not found",
				zap.String("entitlement_key", entKey), zap.Int("index", i))
			continue
		}
		principalId := v2.ResourceId_builder{
			ResourceType: userResourceType.GetId(),
			Resource:     g.PrincipalID,
		}.Build()
		c.grantsIndex[g.ResourceID] = append(c.grantsIndex[g.ResourceID],
			sdkGrant.NewGrant(res, g.EntitlementSlug, principalId))
	}

	for i, m := range data.GrantInheritanceMappings {
		res, ok := c.resources[m.InheritedResourceID]
		if !ok {
			l.Warn("baton-file: skipping inheritance mapping, resource not found",
				zap.String("resource_id", m.InheritedResourceID), zap.Int("index", i))
			continue
		}
		if m.InheritanceDepth != "full" && m.InheritanceDepth != "shallow" {
			l.Warn("baton-file: invalid inheritance_depth, must be \"full\" or \"shallow\"",
				zap.String("value", m.InheritanceDepth), zap.Int("index", i))
			continue
		}
		principalResource, ok := c.resources[m.PrincipalResourceID]
		if !ok {
			l.Warn("baton-file: skipping inheritance mapping, principal resource not found",
				zap.String("principal_resource_id", m.PrincipalResourceID), zap.Int("index", i))
			continue
		}
		membershipKey := fmt.Sprintf("%s:%s", m.PrincipalResourceID, m.PrincipalEntitlementSlug)
		membershipEntitlement, ok := c.entitlements[membershipKey]
		if !ok {
			l.Warn("baton-file: skipping inheritance mapping, membership entitlement not found",
				zap.String("membership_key", membershipKey), zap.Int("index", i))
			continue
		}
		inheritedEntKey := fmt.Sprintf("%s:%s", m.InheritedResourceID, m.InheritedEntitlementSlug)
		if _, ok := c.entitlements[inheritedEntKey]; !ok {
			l.Warn("baton-file: skipping inheritance mapping, inherited entitlement not found",
				zap.String("inherited_entitlement_key", inheritedEntKey), zap.Int("index", i))
			continue
		}
		expandable := v2.GrantExpandable_builder{
			EntitlementIds: []string{membershipEntitlement.GetId()},
			Shallow:        m.InheritanceDepth == "shallow",
		}.Build()
		c.grantsIndex[m.InheritedResourceID] = append(c.grantsIndex[m.InheritedResourceID],
			sdkGrant.NewGrant(res, m.InheritedEntitlementSlug, principalResource.GetId(),
				sdkGrant.WithAnnotation(expandable)))
	}

	// Sort grants once for stable pagination.
	// Primary key: principal type + ID ("user/alice" < "user/bob").
	// Tiebreaker: entitlement ID, so two grants to the same principal always
	// appear in the same order across pages.
	for resourceID := range c.grantsIndex {
		grants := c.grantsIndex[resourceID]
		sort.SliceStable(grants, func(left, right int) bool {
			leftPrincipal := grants[left].GetPrincipal().GetId()
			rightPrincipal := grants[right].GetPrincipal().GetId()
			if leftPrincipal.GetResourceType() != rightPrincipal.GetResourceType() {
				return leftPrincipal.GetResourceType() < rightPrincipal.GetResourceType()
			}
			if leftPrincipal.GetResource() != rightPrincipal.GetResource() {
				return leftPrincipal.GetResource() < rightPrincipal.GetResource()
			}
			return grants[left].GetEntitlement().GetId() < grants[right].GetEntitlement().GetId()
		})
	}

	// 8. Pre-build list index so List() is an O(1) lookup instead of a full map scan.
	// Key is typeID for top-level resources (e.g. "group"), or
	// typeID+"/"+parentType+"/"+parentID for children (e.g. "team/org/org1").
	// Sorted by resource ID for stable pagination across pages.
	c.listIndex = make(map[string][]*v2.Resource)
	for _, res := range c.resources {
		key := listKey(res.GetId().GetResourceType(), res.GetParentResourceId())
		c.listIndex[key] = append(c.listIndex[key], res)
	}
	for key := range c.listIndex {
		slice := c.listIndex[key]
		sort.SliceStable(slice, func(i, j int) bool {
			return slice[i].GetId().GetResource() < slice[j].GetId().GetResource()
		})
	}

	// 9. Pre-build entitlements index keyed by resource ID. Entitlements() becomes
	// an O(1) lookup instead of a full map scan per page.
	// All entitlements for the same resource are grouped together (e.g. "eng:admin"
	// and "eng:member" both land under entIndex["eng"]), sorted by slug for stable pagination.
	c.entIndex = make(map[string][]*v2.Entitlement)
	for _, ent := range c.entitlements {
		resID := ent.GetResource().GetId().GetResource()
		c.entIndex[resID] = append(c.entIndex[resID], ent)
	}
	for resID := range c.entIndex {
		slice := c.entIndex[resID]
		sort.SliceStable(slice, func(i, j int) bool {
			return slice[i].GetSlug() < slice[j].GetSlug()
		})
	}

	return c, nil
}
