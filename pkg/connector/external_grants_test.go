package connector

import (
	"context"
	"testing"

	"github.com/conductorone/baton-file/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// externalGrantFixture returns LoadedData with one app resource, one
// entitlement, and the given external grants.
func externalGrantFixture(grants ...client.ExternalGrant) *client.LoadedData {
	return &client.LoadedData{
		Resources: []client.ResourceData{
			{ID: "payroll-app", ResourceType: "app", Trait: "app", DisplayName: "Payroll"},
		},
		Entitlements: []client.EntitlementData{
			{ResourceID: "payroll-app", EntitlementSlug: "admin", DisplayName: "Admin"},
		},
		ExternalGrants: grants,
	}
}

// findAnnotation unmarshals the first annotation matching target's type into
// target and reports whether one was found.
func findAnnotation(t *testing.T, grant *v2.Grant, target proto.Message) bool {
	t.Helper()
	for _, ann := range grant.GetAnnotations() {
		if ann.MessageIs(target) {
			require.NoError(t, ann.UnmarshalTo(target))
			return true
		}
	}
	return false
}

func TestGrants_ExternalGrant_AttributeMatch(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(client.ExternalGrant{
		ResourceID:        "payroll-app",
		EntitlementSlug:   "admin",
		MatchType:         "attribute",
		MatchResourceType: "user",
		MatchKey:          "email",
		MatchValue:        "jane@corp.com",
	})
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Len(t, grants, 1)

	var match v2.ExternalResourceMatch
	require.True(t, findAnnotation(t, grants[0], &match),
		"external grant must carry ExternalResourceMatch annotation")
	require.Equal(t, v2.ResourceType_TRAIT_USER, match.GetResourceType())
	require.Equal(t, "email", match.GetKey())
	require.Equal(t, "jane@corp.com", match.GetValue())

	// Placeholder principal is synthetic but deterministic.
	require.Equal(t, "user", grants[0].GetPrincipal().GetId().GetResourceType())
	require.Equal(t, "external-email-jane@corp.com", grants[0].GetPrincipal().GetId().GetResource())
}

func TestGrants_ExternalGrant_MatchAll(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(client.ExternalGrant{
		ResourceID:        "payroll-app",
		EntitlementSlug:   "admin",
		MatchType:         "all",
		MatchResourceType: "group",
	})
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Len(t, grants, 1)

	var matchAll v2.ExternalResourceMatchAll
	require.True(t, findAnnotation(t, grants[0], &matchAll),
		"external grant must carry ExternalResourceMatchAll annotation")
	require.Equal(t, v2.ResourceType_TRAIT_GROUP, matchAll.GetResourceType())
	require.Equal(t, "external-all-group", grants[0].GetPrincipal().GetId().GetResource())
}

func TestGrants_ExternalGrant_MatchID(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(client.ExternalGrant{
		ResourceID:        "payroll-app",
		EntitlementSlug:   "admin",
		MatchType:         "id",
		MatchResourceType: "user",
		MatchID:           "okta-user-123",
	})
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Len(t, grants, 1)

	var matchID v2.ExternalResourceMatchID
	require.True(t, findAnnotation(t, grants[0], &matchID),
		"external grant must carry ExternalResourceMatchID annotation")
	require.Equal(t, "okta-user-123", matchID.GetId())
	require.Equal(t, "okta-user-123", grants[0].GetPrincipal().GetId().GetResource())
}

func TestGrants_ExternalGrant_Expansion(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(client.ExternalGrant{
		ResourceID:            "payroll-app",
		EntitlementSlug:       "admin",
		MatchType:             "id",
		MatchResourceType:     "group",
		MatchID:               "ext-group-1",
		ExpandEntitlementSlug: "member",
		ExpandDepth:           "shallow",
	})
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Len(t, grants, 1)

	var matchID v2.ExternalResourceMatchID
	require.True(t, findAnnotation(t, grants[0], &matchID))
	require.Equal(t, "ext-group-1", matchID.GetId())

	// The expandable must reference the placeholder principal's entitlement in
	// bid format — the SDK re-anchors the slug onto the matched group.
	var expandable v2.GrantExpandable
	require.True(t, findAnnotation(t, grants[0], &expandable),
		"expand_entitlement_slug must add a GrantExpandable annotation")
	require.Equal(t, []string{"bid:e:group/ext-group-1:member"}, expandable.GetEntitlementIds())
	require.True(t, expandable.GetShallow(), "expand_depth=shallow must set Shallow=true")
}

func TestGrants_ExternalGrant_Expansion_DefaultDepthIsFull(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(client.ExternalGrant{
		ResourceID:            "payroll-app",
		EntitlementSlug:       "admin",
		MatchType:             "attribute",
		MatchResourceType:     "group",
		MatchKey:              "name",
		MatchValue:            "External Group",
		ExpandEntitlementSlug: "member",
	})
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Len(t, grants, 1)

	var expandable v2.GrantExpandable
	require.True(t, findAnnotation(t, grants[0], &expandable))
	require.False(t, expandable.GetShallow(), "omitted expand_depth must default to full (Shallow=false)")
}

func TestGrants_ExternalGrant_Expansion_SkipsInvalidCombos(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(
		// Expansion with match_type "all" is not supported by the SDK.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "all", MatchResourceType: "group",
			ExpandEntitlementSlug: "member",
		},
		// Expansion on a user principal is not supported.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "id", MatchResourceType: "user", MatchID: "u1",
			ExpandEntitlementSlug: "member",
		},
		// Invalid expand_depth.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "id", MatchResourceType: "group", MatchID: "g1",
			ExpandEntitlementSlug: "member", ExpandDepth: "deep",
		},
	)
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Empty(t, grants, "invalid expansion combinations must be skipped")
}

func TestGrants_ExternalGrant_SkipsInvalidRows(t *testing.T) {
	ctx := context.Background()
	data := externalGrantFixture(
		// Unknown local resource.
		client.ExternalGrant{
			ResourceID: "nope", EntitlementSlug: "admin",
			MatchType: "all", MatchResourceType: "user",
		},
		// Unknown entitlement.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "nope",
			MatchType: "all", MatchResourceType: "user",
		},
		// Bad match_resource_type.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "all", MatchResourceType: "role",
		},
		// Bad match_type.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "fuzzy", MatchResourceType: "user",
		},
		// Missing match_id.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "id", MatchResourceType: "user",
		},
		// Missing match_value.
		client.ExternalGrant{
			ResourceID: "payroll-app", EntitlementSlug: "admin",
			MatchType: "attribute", MatchResourceType: "user", MatchKey: "email",
		},
	)
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := testBuilder(cache, cache.resourceTypes["app"])
	grants := allGrants(t, b, cache.resources["payroll-app"])
	require.Empty(t, grants, "invalid external grant rows must be skipped, not emitted")
}
