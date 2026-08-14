package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/conductorone/baton-file/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/bid"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type FileConnector struct {
	inputFilePath string
	cache         cacheHolder

	// refreshMu serializes the read → build → publish sequence (refresh).
	// The holder's atomic pointer keeps readers lock-free, but without this
	// lock two concurrent Validate() calls (a health probe racing a sync
	// start across two file edits) could publish out of order and leave the
	// holder one generation stale until the next Validate(). A
	// CompareAndSwap loop cannot fix that: generations are content hashes
	// with no ordering, so on CAS failure there is no way to tell which
	// build is newer. Serializing the whole sequence makes publish order
	// follow file-read order, and also prevents two full cache builds from
	// running concurrently (which would triple peak memory).
	refreshMu sync.Mutex

	// registeredTypes records the resource type IDs registered by the
	// construction-time ResourceSyncers() call. Written once during
	// construction (before the gRPC server starts serving) and read-only
	// afterwards, so it needs no lock. Validate() uses it to warn when a
	// file edit introduces types that have no registered syncer and
	// therefore will not sync until restart.
	registeredTypes map[string]struct{}
}

// refresh re-reads the input file and publishes a fresh cache when its
// contents changed, returning the cache now being served and whether it was
// republished. On error nothing is stored, so the previously published cache
// keeps serving last-known-good data.
func (fc *FileConnector) refresh(ctx context.Context) (*syncCache, bool, error) {
	fc.refreshMu.Lock()
	defer fc.refreshMu.Unlock()

	previous := fc.cache.load()
	cache, err := loadValidatedCache(ctx, fc.inputFilePath, previous)
	if err != nil {
		return nil, false, err
	}
	if cache == previous {
		return cache, false, nil
	}
	fc.cache.store(cache)
	return cache, true, nil
}

// cacheHolder shares the live syncCache between the FileConnector and every
// resourceBuilder, so the cache built from the current file contents can be
// swapped in atomically on each sync.
//
// IMPORTANT — hot-load contract (do not break):
// The SDK constructs the connector and calls ResourceSyncers() exactly ONCE
// per process, before its task loop starts. In a long-running service the
// only per-sync hook this connector gets is Validate(), so Validate() is
// where the input file is re-read and the fresh syncCache is published here.
// Data-section changes to the file MUST be picked up on the next sync without
// a restart; only schema changes (a new resource type, or a trait change to
// an existing type) require one. Do NOT capture a *syncCache snapshot in a
// builder, and do NOT assume the SDK refreshes anything between syncs (it
// does not — that assumption caused the original hot-load regression in
// PR #40). TestHotReload_DataChangesPickedUpBySync enforces this contract.
//
// This is deliberate instance state, a documented deviation from the skills'
// stateless-connector rule: that rule assumes a remote API as the source of
// truth, whereas here the file IS the source of truth and is re-read on every
// sync, so a cold start is identical to a warm one. The swap normally happens
// at sync start (Validate), but the SDK's health-check endpoint also calls
// Validate and can swap mid-sync; page tokens are therefore stamped with the
// cache generation (syncCache.gen) so an in-flight listing restarts instead
// of resuming into a different generation's offsets. During a swap both old
// and new caches are briefly live (~2x peak memory for the dataset).
//
// Known limit: generation stamps make each individual listing consistent,
// not a whole sync. If the file genuinely changes mid-sync and a health
// probe swaps the cache, phases that already ran (e.g. List) reflect the old
// generation while later phases (e.g. Grants) reflect the new one — a
// one-sync inconsistency (such as a grant whose principal was never listed)
// that the next sync heals. The connector cannot pin a cache per sync: a
// probe Validate and a sync-start Validate arrive as the same RPC, and no
// sync-lifecycle hook exists. The fingerprint short-circuit in
// loadValidatedCache confines this to actual content changes — unchanged
// files never swap.
//
// Known limit: hot-load requires a successful start. ResourceSyncers()
// performs the first load at construction; if the file is INVALID at that
// moment, zero resource types are registered for the process lifetime and
// every sync fails until the file is fixed AND the process restarts (see
// ResourceSyncers). TestHotReload_InvalidFileAtStartupRequiresRestart pins
// this behavior. A file that is valid but has no data rows is NOT this trap:
// it is a legitimate empty source of truth — ResourceSyncers registers the
// standard trait types so syncs succeed (empty) and later data rows
// hot-load; TestHotReload_EmptyFileAtStartupSyncsAndHotLoads pins that.
type cacheHolder struct {
	p atomic.Pointer[syncCache]
}

// load is nil-receiver-safe because StaticCapabilitiesConnector builds
// resourceBuilders without a holder.
func (h *cacheHolder) load() *syncCache {
	if h == nil {
		return nil
	}
	return h.p.Load()
}

func (h *cacheHolder) store(cache *syncCache) { h.p.Store(cache) }

func (fc *FileConnector) Close() error { return nil }

// StaticCapabilitiesConnector used exclusively by the capabilities sub-command via WithDefaultCapabilitiesConnectorBuilderV2.
// It reports all resource types supported by the connector (one per TraitMap entry) without reading any file.
type StaticCapabilitiesConnector struct{}

func (s *StaticCapabilitiesConnector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "File Connector",
		Description: "Connector that syncs identity and access data from a local file",
	}, nil
}

func (s *StaticCapabilitiesConnector) Validate(ctx context.Context) (annotations.Annotations, error) {
	return nil, nil
}

func (s *StaticCapabilitiesConnector) Close() error { return nil }

func (s *StaticCapabilitiesConnector) ResourceSyncers(_ context.Context) []connectorbuilder.ResourceSyncerV2 {
	var syncers []connectorbuilder.ResourceSyncerV2
	for name := range TraitMap {
		syncers = append(syncers, &resourceBuilder{
			resourceType: buildDynamicResourceType(name, name),
		})
	}
	return syncers
}

type syncCache struct {
	// gen fingerprints the file contents this cache was built from; it is
	// stamped into page tokens so a listing resumed against a different
	// generation restarts instead of replaying offsets into changed slices
	// (see paginate). Empty for caches built directly in tests.
	gen string

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

// Depth values shared by grant_inheritance_mappings (inheritance_depth) and
// external_grants (expand_depth).
const (
	depthFull    = "full"
	depthShallow = "shallow"
)

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

	// Re-read, validate, and publish the file's current contents. Validate()
	// runs at the start of every sync, so this is the hot-load refresh point
	// (see cacheHolder).
	cache, changed, err := fc.refresh(ctx)
	if err != nil {
		return nil, err
	}
	if changed {
		// Debug, not Info: this fires on every sync AND every health-check
		// probe, so Info would flood default logs on probed deployments.
		ctxzap.Extract(ctx).Debug("baton-file: refreshed data from input file",
			zap.Int("resource_types", len(cache.resourceTypes)),
			zap.Int("resources", len(cache.resources)))

		// Schema drift is otherwise silent: rows whose resource type was not
		// registered at startup simply never sync. Warn so the operator
		// knows a restart is needed for them. registeredTypes is nil only
		// when ResourceSyncers() has not run (unit tests calling Validate
		// directly); skip the check there.
		if fc.registeredTypes != nil {
			var missing []string
			for typeID := range cache.resourceTypes {
				if _, ok := fc.registeredTypes[typeID]; !ok {
					missing = append(missing, typeID)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				ctxzap.Extract(ctx).Warn(
					"baton-file: file contains resource types not registered at startup; restart the service to sync them",
					zap.Strings("resource_types", missing))
			}
		}
	}

	return nil, nil
}

// loadValidatedCache reads the input file, runs all cross-record validations,
// and builds the derived sync cache from it. When the file's content
// fingerprint matches the current cache's generation, current is returned
// unchanged: the rebuild would be identical, and skipping it both avoids
// redundant work on health-check probes and keeps no-op Validate calls from
// swapping the pointer under an in-flight sync.
func loadValidatedCache(ctx context.Context, inputFilePath string, current *syncCache) (*syncCache, error) {
	// Fingerprint the raw bytes to stamp this build's generation into page
	// tokens (see paginate). Hashing raw content is format-agnostic and
	// exact, and the extra read is cheap next to parsing — the loaders take
	// paths, not bytes, so reusing one read would mean restructuring them. A
	// file edit racing between the two reads mislabels one build, which at
	// worst restarts a listing.
	raw, err := os.ReadFile(inputFilePath)
	if err != nil {
		return nil, fmt.Errorf("baton-file: failed to read input file: %w", err)
	}
	sum := sha256.Sum256(raw)
	gen := hex.EncodeToString(sum[:8])

	if current != nil && current.gen == gen {
		return current, nil
	}

	data, err := client.LoadFileData(inputFilePath)
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

	cache, err := newSyncCache(ctx, data)
	if err != nil {
		return nil, err
	}
	cache.gen = gen
	return cache, nil
}

func (fc *FileConnector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncerV2 {
	l := ctxzap.Extract(ctx)

	// IMPORTANT: the SDK calls this exactly ONCE per process, inside
	// connectorbuilder.NewConnector at construction (vendor/.../
	// connectorbuilder/connectorbuilder.go:216), BEFORE any Validate() —
	// this is the FIRST load of the file, not a fallback. The set of
	// resource types registered here is fixed for the process lifetime;
	// Validate() then refreshes the data behind fc.cache on every sync. See
	// cacheHolder for the hot-load contract.
	cache, _, err := fc.refresh(ctx)
	if err != nil {
		// The signature cannot return an error, so log-and-return-nil is
		// all that is possible — and the consequence is severe: with zero
		// syncers registered, every sync fails with FailedPrecondition
		// ("no resource builders found", vendor/.../connectorbuilder/
		// resource_syncer.go:102) even after the operator fixes the file.
		// Recovering from a bad file AT STARTUP requires a process restart;
		// hot-load only helps a connector that started with a valid file.
		// Warn, not Error: Validate() returns the real error to the SDK on
		// every sync, so this log is a secondary signal, and house rules
		// classify input failures as Warn.
		l.Warn("baton-file: failed to load input file at startup; syncs will fail until the file is fixed and the service is restarted",
			zap.Error(err))
		return nil
	}

	var syncers []connectorbuilder.ResourceSyncerV2
	if len(cache.resourceTypes) == 0 {
		// A valid file with no data rows is a legitimate state — an empty
		// source of truth — not an error. Register the standard trait types
		// (the same set StaticCapabilitiesConnector declares as this
		// connector's capabilities) so syncs succeed, emit nothing, and
		// data rows added to the file later hot-load without a restart.
		// Rows using CUSTOM type IDs still need a restart, like any schema
		// change; Validate() warns when it sees them.
		for name := range TraitMap {
			syncers = append(syncers, &resourceBuilder{
				cache:        &fc.cache,
				resourceType: buildDynamicResourceType(name, name),
			})
		}
		l.Info("baton-file: input file has no data rows; registered standard resource types",
			zap.Int("count", len(syncers)))
	} else {
		for _, rt := range cache.resourceTypes {
			// resourceType is INTENTIONALLY frozen at registration time and
			// never refreshed from later cache generations. Hot-load covers
			// the file's data sections only; schema — the set of resource
			// types and their traits — is fixed for the process lifetime
			// because the SDK registers syncers by type exactly once.
			// Editing an existing type's trait in the file therefore
			// requires a restart, same as adding a new type; resolving rt
			// from the live cache here would not change that, since the
			// SDK-side registration would still hold the startup-time type.
			syncers = append(syncers, &resourceBuilder{cache: &fc.cache, resourceType: rt})
		}
		l.Info("baton-file: created resource syncers", zap.Int("count", len(syncers)))
	}

	fc.registeredTypes = make(map[string]struct{}, len(syncers))
	for _, s := range syncers {
		if rb, ok := s.(*resourceBuilder); ok {
			fc.registeredTypes[rb.resourceType.GetId()] = struct{}{}
		}
	}
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
			l.Debug("baton-file: skipping direct grant, resource not found",
				zap.String("resource_id", g.ResourceID), zap.Int("index", i))
			continue
		}
		principalRes, ok := c.resources[g.PrincipalID]
		if !ok {
			l.Debug("baton-file: skipping direct grant, principal id not found",
				zap.String("principal_id", g.PrincipalID), zap.Int("index", i))
			continue
		}
		if principalRes.GetId().GetResourceType() != userResourceType.GetId() {
			l.Debug("baton-file: skipping direct grant, principal is not a user",
				zap.String("principal_id", g.PrincipalID),
				zap.String("actual_type", principalRes.GetId().GetResourceType()),
				zap.Int("index", i))
			continue
		}
		entKey := fmt.Sprintf("%s:%s", g.ResourceID, g.EntitlementSlug)
		if _, ok := c.entitlements[entKey]; !ok {
			l.Debug("baton-file: skipping direct grant, entitlement not found",
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
			l.Debug("baton-file: skipping inheritance mapping, resource not found",
				zap.String("resource_id", m.InheritedResourceID), zap.Int("index", i))
			continue
		}
		if m.InheritanceDepth != depthFull && m.InheritanceDepth != depthShallow {
			l.Debug("baton-file: invalid inheritance_depth, must be \"full\" or \"shallow\"",
				zap.String("value", m.InheritanceDepth), zap.Int("index", i))
			continue
		}
		principalResource, ok := c.resources[m.PrincipalResourceID]
		if !ok {
			l.Debug("baton-file: skipping inheritance mapping, principal resource not found",
				zap.String("principal_resource_id", m.PrincipalResourceID), zap.Int("index", i))
			continue
		}
		membershipKey := fmt.Sprintf("%s:%s", m.PrincipalResourceID, m.PrincipalEntitlementSlug)
		membershipEntitlement, ok := c.entitlements[membershipKey]
		if !ok {
			l.Debug("baton-file: skipping inheritance mapping, membership entitlement not found",
				zap.String("membership_key", membershipKey), zap.Int("index", i))
			continue
		}
		inheritedEntKey := fmt.Sprintf("%s:%s", m.InheritedResourceID, m.InheritedEntitlementSlug)
		if _, ok := c.entitlements[inheritedEntKey]; !ok {
			l.Debug("baton-file: skipping inheritance mapping, inherited entitlement not found",
				zap.String("inherited_entitlement_key", inheritedEntKey), zap.Int("index", i))
			continue
		}
		expandable := v2.GrantExpandable_builder{
			EntitlementIds: []string{membershipEntitlement.GetId()},
			Shallow:        m.InheritanceDepth == depthShallow,
		}.Build()
		c.grantsIndex[m.InheritedResourceID] = append(c.grantsIndex[m.InheritedResourceID],
			sdkGrant.NewGrant(res, m.InheritedEntitlementSlug, principalResource.GetId(),
				sdkGrant.WithAnnotation(expandable)))
	}

	// External grants: the principal lives in another connector (the shared
	// identity source), not in this file. Each grant is emitted with a
	// synthetic placeholder principal plus an ExternalResourceMatch* annotation
	// describing how to find the real principal. During sync the SDK matches
	// the annotation against the identity source's principals, creates one
	// real grant per match, and deletes the placeholder — so the placeholder
	// principal ID only needs to be deterministic and unique per grant.
	for i, eg := range data.ExternalGrants {
		res, ok := c.resources[eg.ResourceID]
		if !ok {
			l.Debug("baton-file: skipping external grant, resource not found",
				zap.String("resource_id", eg.ResourceID), zap.Int("index", i))
			continue
		}
		entKey := fmt.Sprintf("%s:%s", eg.ResourceID, eg.EntitlementSlug)
		if _, ok := c.entitlements[entKey]; !ok {
			l.Debug("baton-file: skipping external grant, entitlement not found",
				zap.String("entitlement_key", entKey), zap.Int("index", i))
			continue
		}
		matchResourceType := strings.ToLower(eg.MatchResourceType)
		if matchResourceType != "user" && matchResourceType != "group" {
			l.Debug("baton-file: skipping external grant, match_resource_type must be \"user\" or \"group\"",
				zap.String("match_resource_type", eg.MatchResourceType), zap.Int("index", i))
			continue
		}
		trait := TraitMap[matchResourceType]

		var matchAnno proto.Message
		var placeholderID string
		switch strings.ToLower(eg.MatchType) {
		case "all":
			matchAnno = v2.ExternalResourceMatchAll_builder{ResourceType: trait}.Build()
			placeholderID = "external-all-" + matchResourceType
		case "id":
			if eg.MatchID == "" {
				l.Debug("baton-file: skipping external grant, match_type \"id\" requires match_id",
					zap.Int("index", i))
				continue
			}
			matchAnno = v2.ExternalResourceMatchID_builder{Id: eg.MatchID}.Build()
			placeholderID = eg.MatchID
		case "attribute":
			if eg.MatchKey == "" || eg.MatchValue == "" {
				l.Debug("baton-file: skipping external grant, match_type \"attribute\" requires match_key and match_value",
					zap.Int("index", i))
				continue
			}
			matchAnno = v2.ExternalResourceMatch_builder{
				ResourceType: trait,
				Key:          eg.MatchKey,
				Value:        eg.MatchValue,
			}.Build()
			placeholderID = fmt.Sprintf("external-%s-%s", eg.MatchKey, eg.MatchValue)
		default:
			l.Debug("baton-file: skipping external grant, invalid match_type, must be \"all\", \"id\", or \"attribute\"",
				zap.String("match_type", eg.MatchType), zap.Int("index", i))
			continue
		}

		placeholderPrincipal := v2.ResourceId_builder{
			ResourceType: matchResourceType,
			Resource:     placeholderID,
		}.Build()

		grantAnnos := []proto.Message{matchAnno}

		// Optional grant expansion: members of the matched external group
		// inherit this grant through the group's named entitlement. The
		// GrantExpandable must reference the entitlement in bid format,
		// anchored to the placeholder principal — during matching the SDK
		// re-anchors the slug onto the matched group and verifies that
		// entitlement exists in the store. Only supported for group
		// principals matched by "id" or "attribute" (the SDK does not
		// rewrite expandables for "all" matches).
		if eg.ExpandEntitlementSlug != "" {
			if matchResourceType != "group" || strings.ToLower(eg.MatchType) == "all" {
				l.Debug("baton-file: skipping external grant, expansion requires match_resource_type \"group\" and match_type \"id\" or \"attribute\"",
					zap.String("match_type", eg.MatchType),
					zap.String("match_resource_type", eg.MatchResourceType),
					zap.Int("index", i))
				continue
			}
			if eg.ExpandDepth != "" && eg.ExpandDepth != depthFull && eg.ExpandDepth != depthShallow {
				l.Debug("baton-file: skipping external grant, invalid expand_depth, must be \"full\" or \"shallow\"",
					zap.String("expand_depth", eg.ExpandDepth), zap.Int("index", i))
				continue
			}
			entBid, err := bid.MakeBid(v2.Entitlement_builder{
				Resource: v2.Resource_builder{Id: placeholderPrincipal}.Build(),
				Slug:     eg.ExpandEntitlementSlug,
			}.Build())
			if err != nil {
				l.Warn("baton-file: skipping external grant, failed to build expandable entitlement bid",
					zap.Int("index", i), zap.Error(err))
				continue
			}
			grantAnnos = append(grantAnnos, v2.GrantExpandable_builder{
				EntitlementIds: []string{entBid},
				Shallow:        eg.ExpandDepth == depthShallow,
			}.Build())
		}

		c.grantsIndex[eg.ResourceID] = append(c.grantsIndex[eg.ResourceID],
			sdkGrant.NewGrant(res, eg.EntitlementSlug, placeholderPrincipal,
				sdkGrant.WithAnnotation(grantAnnos...)))
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
