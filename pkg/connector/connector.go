package connector

import (
	"context"
	"fmt"
	"os"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-file/pkg/client"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type FileConnector struct {
	inputFilePath  string
	validatedData  *client.LoadedData
}

func (fc *FileConnector) Close() error { return nil }

type syncCache struct {
	loadedData    *client.LoadedData
	resourceTypes map[string]*v2.ResourceType
	resources     map[string]*v2.Resource
	entitlements  map[string]*v2.Entitlement
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
	fc.validatedData = data

	return nil, nil
}

func (fc *FileConnector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncerV2 {
	l := ctxzap.Extract(ctx)

	loadedData := fc.validatedData
	if loadedData == nil {
		var err error
		loadedData, err = client.LoadFileData(fc.inputFilePath)
		if err != nil {
			l.Error("baton-file: failed to load input file", zap.Error(err))
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
	c := &syncCache{loadedData: data}

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

	// 2. Build user resources
	c.resources = make(map[string]*v2.Resource)
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
			l.Error("baton-file: failed to build user resource", zap.String("id", u.ID), zap.Error(err))
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
			l.Error("baton-file: resource type not found", zap.String("id", r.ID), zap.String("type", r.ResourceType))
			continue
		}
		res, err := buildResource(ctx, r, rt)
		if err != nil {
			l.Error("baton-file: failed to build resource", zap.String("id", r.ID), zap.Error(err))
			continue
		}
		c.resources[r.ID] = res
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
		if err := rs.WithParentResourceID(parent.GetId())(child); err != nil {
			l.Error("baton-file: failed to set parent resource id",
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
			l.Error("baton-file: parent resource for entitlement not found", zap.String("resource_id", e.ResourceID))
			continue
		}
		ent := entitlement.NewAssignmentEntitlement(parentRes, e.EntitlementSlug,
			entitlement.WithDisplayName(e.DisplayName),
			entitlement.WithDescription(e.Description),
		)
		key := fmt.Sprintf("%s:%s", e.ResourceID, e.EntitlementSlug)
		c.entitlements[key] = ent
	}

	return c, nil
}
