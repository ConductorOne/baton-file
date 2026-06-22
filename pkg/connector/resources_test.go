package connector

import (
	"context"
	"fmt"
	"testing"

	"github.com/conductorone/baton-file/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/stretchr/testify/require"
)

// allGrants pages through Grants() until exhausted and returns every grant.
func allGrants(t *testing.T, b *resourceBuilder, res *v2.Resource) []*v2.Grant {
	t.Helper()
	ctx := context.Background()
	var out []*v2.Grant
	token := ""
	for {
		opts := rs.SyncOpAttrs{PageToken: pagination.Token{Token: token}}
		page, results, err := b.Grants(ctx, res, opts)
		require.NoError(t, err)
		out = append(out, page...)
		if token = results.NextPageToken; token == "" {
			break
		}
	}
	return out
}

// allEntitlements pages through Entitlements() until exhausted.
func allEntitlements(t *testing.T, b *resourceBuilder, res *v2.Resource) []*v2.Entitlement {
	t.Helper()
	ctx := context.Background()
	var out []*v2.Entitlement
	token := ""
	for {
		opts := rs.SyncOpAttrs{PageToken: pagination.Token{Token: token}}
		page, results, err := b.Entitlements(ctx, res, opts)
		require.NoError(t, err)
		out = append(out, page...)
		if token = results.NextPageToken; token == "" {
			break
		}
	}
	return out
}

// allResources pages through List() until exhausted.
func allResources(t *testing.T, b *resourceBuilder, parent *v2.ResourceId) []*v2.Resource {
	t.Helper()
	ctx := context.Background()
	var out []*v2.Resource
	token := ""
	for {
		opts := rs.SyncOpAttrs{PageToken: pagination.Token{Token: token}}
		page, results, err := b.List(ctx, parent, opts)
		require.NoError(t, err)
		out = append(out, page...)
		if token = results.NextPageToken; token == "" {
			break
		}
	}
	return out
}

// paginate() unit tests — clampPageSize and paginate are new functions written for this fix.

func TestClampPageSize(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{"zero uses default", 0, pageSize},
		{"negative uses default", -5, pageSize},
		{"within range unchanged", 250, 250},
		{"at max unchanged", pageSize, pageSize},
		{"above max clamped", pageSize + 500, pageSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, clampPageSize(tt.requested))
		})
	}
}

func TestPaginate_Exhaustion(t *testing.T) {
	const total = 2500
	items := make([]int, total)
	for i := range items {
		items[i] = i
	}
	seen := make(map[int]struct{}, total)
	token := ""
	pages := 0
	for {
		page, next, err := paginate(items, token, 0)
		require.NoError(t, err)
		require.LessOrEqual(t, len(page), pageSize)
		pages++
		for _, v := range page {
			_, dup := seen[v]
			require.False(t, dup, "item %d returned on more than one page", v)
			seen[v] = struct{}{}
		}
		if token = next; token == "" {
			break
		}
		require.Less(t, pages, total, "pagination did not terminate")
	}
	require.Equal(t, total, len(seen))
	require.Equal(t, (total+pageSize-1)/pageSize, pages)
}

func TestPaginate_StaleOffset(t *testing.T) {
	items := make([]int, 50)
	page, next, err := paginate(items, "200", 0)
	require.NoError(t, err)
	require.Empty(t, page)
	require.Empty(t, next)
}

func TestPaginate_InvalidToken(t *testing.T) {
	items := []int{1, 2, 3}
	for _, tok := range []string{"not-a-number", "abc", "-1"} {
		_, _, err := paginate(items, tok, 0)
		require.Error(t, err, "token %q should return error", tok)
	}
}

// Grants() pagination tests.

func TestGrants_PaginatesCorrectly(t *testing.T) {
	ctx := context.Background()
	const total = 2500
	data := &client.LoadedData{
		Users:            make([]client.UserData, total),
		Resources:        []client.ResourceData{{ID: "res1", ResourceType: "group", Trait: "group", DisplayName: "Big Group"}},
		Entitlements:     []client.EntitlementData{{ResourceID: "res1", EntitlementSlug: "member", DisplayName: "Member"}},
		DirectUserGrants: make([]client.DirectUserGrant, total),
	}
	for i := range total {
		uid := fmt.Sprintf("u%05d", i)
		data.Users[i] = client.UserData{ID: uid, DisplayName: uid}
		data.DirectUserGrants[i] = client.DirectUserGrant{PrincipalID: uid, ResourceID: "res1", EntitlementSlug: "member"}
	}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["group"]}
	grants := allGrants(t, b, cache.resources["res1"])

	require.Len(t, grants, total)
	seen := make(map[string]struct{}, total)
	for _, g := range grants {
		key := g.GetPrincipal().GetId().GetResource()
		_, dup := seen[key]
		require.False(t, dup, "duplicate grant for principal %s", key)
		seen[key] = struct{}{}
	}
}

func TestGrants_InvalidPageToken(t *testing.T) {
	ctx := context.Background()
	data := &client.LoadedData{
		Users:            []client.UserData{{ID: "u1", DisplayName: "U1"}},
		Resources:        []client.ResourceData{{ID: "res1", ResourceType: "group", Trait: "group", DisplayName: "G"}},
		Entitlements:     []client.EntitlementData{{ResourceID: "res1", EntitlementSlug: "member", DisplayName: "M"}},
		DirectUserGrants: []client.DirectUserGrant{{PrincipalID: "u1", ResourceID: "res1", EntitlementSlug: "member"}},
	}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["group"]}
	res := cache.resources["res1"]

	for _, tok := range []string{"not-a-number", "-1"} {
		opts := rs.SyncOpAttrs{PageToken: pagination.Token{Token: tok}}
		_, _, err := b.Grants(ctx, res, opts)
		require.Error(t, err, "token %q should return error", tok)
	}
}

// Entitlements() pagination tests.

func TestEntitlements_PaginatesCorrectly(t *testing.T) {
	ctx := context.Background()
	const total = 1500
	entsData := make([]client.EntitlementData, total)
	for i := range total {
		entsData[i] = client.EntitlementData{
			ResourceID:      "res1",
			EntitlementSlug: fmt.Sprintf("perm-%04d", i),
			DisplayName:     fmt.Sprintf("Permission %04d", i),
		}
	}
	data := &client.LoadedData{
		Resources:    []client.ResourceData{{ID: "res1", ResourceType: "role", Trait: "role", DisplayName: "Big Role"}},
		Entitlements: entsData,
	}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["role"]}
	ents := allEntitlements(t, b, cache.resources["res1"])

	require.Len(t, ents, total)
	seen := make(map[string]struct{}, total)
	for _, e := range ents {
		_, dup := seen[e.GetSlug()]
		require.False(t, dup, "duplicate entitlement slug %s", e.GetSlug())
		seen[e.GetSlug()] = struct{}{}
	}
}

func TestEntitlements_OnlyForRequestedResource(t *testing.T) {
	ctx := context.Background()
	data := &client.LoadedData{
		Resources: []client.ResourceData{
			{ID: "eng", ResourceType: "group", Trait: "group", DisplayName: "Engineering"},
			{ID: "mkt", ResourceType: "group", Trait: "group", DisplayName: "Marketing"},
		},
		Entitlements: []client.EntitlementData{
			{ResourceID: "eng", EntitlementSlug: "member", DisplayName: "Member"},
			{ResourceID: "mkt", EntitlementSlug: "member", DisplayName: "Member"},
		},
	}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["group"]}
	ents := allEntitlements(t, b, cache.resources["eng"])

	require.Len(t, ents, 1, "must only return entitlements for the requested resource")
	require.Equal(t, "eng", ents[0].GetResource().GetId().GetResource())
}

// List() pagination tests.

func TestList_PaginatesCorrectly(t *testing.T) {
	ctx := context.Background()
	const total = 1500
	resources := make([]client.ResourceData, total)
	for i := range total {
		resources[i] = client.ResourceData{
			ID:           fmt.Sprintf("g%05d", i),
			ResourceType: "group",
			Trait:        "group",
			DisplayName:  fmt.Sprintf("Group %05d", i),
		}
	}
	data := &client.LoadedData{Resources: resources}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["group"]}
	got := allResources(t, b, nil)

	require.Len(t, got, total)
	seen := make(map[string]struct{}, total)
	for _, r := range got {
		_, dup := seen[r.GetId().GetResource()]
		require.False(t, dup, "duplicate resource %s", r.GetId().GetResource())
		seen[r.GetId().GetResource()] = struct{}{}
	}
}

func TestList_ReturnsCorrectResourceType(t *testing.T) {
	ctx := context.Background()
	data := &client.LoadedData{
		Users: []client.UserData{{ID: "alice", DisplayName: "Alice"}},
		Resources: []client.ResourceData{
			{ID: "eng", ResourceType: "group", Trait: "group", DisplayName: "Engineering"},
			{ID: "viewer", ResourceType: "role", Trait: "role", DisplayName: "Viewer"},
		},
	}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["group"]}
	resources := allResources(t, b, nil)

	require.Len(t, resources, 1)
	require.Equal(t, "eng", resources[0].GetId().GetResource())
	require.Equal(t, "group", resources[0].GetId().GetResourceType())
}

func TestList_FiltersByParent(t *testing.T) {
	ctx := context.Background()
	data := &client.LoadedData{
		Resources: []client.ResourceData{
			{ID: "org1", ResourceType: "org", Trait: "group", DisplayName: "Org 1"},
			{ID: "team-a", ResourceType: "team", Trait: "group", DisplayName: "Team A", ParentResource: "org1"},
			{ID: "team-b", ResourceType: "team", Trait: "group", DisplayName: "Team B", ParentResource: "org1"},
			{ID: "team-c", ResourceType: "team", Trait: "group", DisplayName: "Team C"},
		},
	}
	cache, err := newSyncCache(ctx, data)
	require.NoError(t, err)

	b := &resourceBuilder{cache: cache, resourceType: cache.resourceTypes["team"]}

	topLevel := allResources(t, b, nil)
	require.Len(t, topLevel, 1)
	require.Equal(t, "team-c", topLevel[0].GetId().GetResource())

	children := allResources(t, b, &v2.ResourceId{ResourceType: "org", Resource: "org1"})
	require.Len(t, children, 2)
	ids := map[string]bool{children[0].GetId().GetResource(): true, children[1].GetId().GetResource(): true}
	require.True(t, ids["team-a"])
	require.True(t, ids["team-b"])
}
