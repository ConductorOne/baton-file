package connector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/stretchr/testify/require"
)

// Hot-load contract tests (see cacheHolder in connector.go).
//
// The SDK constructs the connector and calls ResourceSyncers() exactly once
// per process, then re-runs Validate() at the start of every sync. In a
// long-running service, data-section changes to the input file MUST be
// served on the next sync without a restart. These tests mirror that
// lifecycle: one ResourceSyncers() call, then file edits followed by
// Validate(), asserting through the ORIGINAL builders.
//
// Unlike the other connector tests, fixtures here are real files written to
// disk rather than in-memory LoadedData: hot-load IS the file → Validate →
// cache path, so bypassing the loaders would test nothing.

const hotReloadCSVBase = `record_type,id,display_name,email,status,type,profile.department,profile.title,resource_type,trait,description,resource_id,entitlement_slug,principal_id
user,alex.taylor,Alex Taylor,alex.taylor@example.com,enabled,human,Engineering,Software Engineer,,,,,,
user,sam.johnson,Sam Johnson,sam.johnson@example.com,enabled,human,Marketing,Marketing Manager,,,,,,
resource,engineering,Engineering,,,,,,team,group,Engineering team,,,
entitlement,,Member,,,,,,,,,engineering,member,
direct_user_grant,,,,,,,,,,,engineering,member,alex.taylor
`

// hotReloadCSVUpdated adds one user and one grant to the base data.
const hotReloadCSVUpdated = hotReloadCSVBase +
	`user,jordan.lee,Jordan Lee,jordan.lee@example.com,enabled,human,Engineering,Engineering Manager,,,,,,
direct_user_grant,,,,,,,,,,,engineering,member,jordan.lee
`

// hotReloadCSVInvalid duplicates an existing user id, which fails
// ValidateUniqueIDs.
const hotReloadCSVInvalid = hotReloadCSVBase +
	`user,alex.taylor,Alex Duplicate,alex.duplicate@example.com,enabled,human,,,,,,,,
`

// startConnector writes the CSV, then walks the once-per-process SDK startup
// sequence (Validate, then ResourceSyncers) and returns the connector plus
// the long-lived builders keyed by resource type id.
func startConnector(t *testing.T, path, csv string) (*FileConnector, map[string]*resourceBuilder) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, os.WriteFile(path, []byte(csv), 0o600))

	fc := &FileConnector{inputFilePath: path}
	_, err := fc.Validate(ctx)
	require.NoError(t, err)

	builders := make(map[string]*resourceBuilder)
	for _, s := range fc.ResourceSyncers(ctx) {
		b, ok := s.(*resourceBuilder)
		require.True(t, ok)
		builders[b.ResourceType(ctx).GetId()] = b
	}
	require.Contains(t, builders, "user")
	require.Contains(t, builders, "team")
	return fc, builders
}

func findResource(t *testing.T, b *resourceBuilder, id string) *v2.Resource {
	t.Helper()
	for _, r := range allResources(t, b, nil) {
		if r.GetId().GetResource() == id {
			return r
		}
	}
	t.Fatalf("resource %q not found", id)
	return nil
}

func TestHotReload_DataChangesPickedUpBySync(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "input.csv")
	fc, builders := startConnector(t, path, hotReloadCSVBase)

	require.Len(t, allResources(t, builders["user"], nil), 2)
	engineering := findResource(t, builders["team"], "engineering")
	require.Len(t, allGrants(t, builders["team"], engineering), 1)

	// The customer edits the file while the service keeps running.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVUpdated), 0o600))

	// Validate() is the only per-sync hook the SDK gives this connector, so
	// it alone must refresh the data the original builders serve.
	_, err := fc.Validate(ctx)
	require.NoError(t, err)

	require.Len(t, allResources(t, builders["user"], nil), 3)
	require.Len(t, allGrants(t, builders["team"], engineering), 2)
}

func TestHotReload_SwapMidListingRestartsPagination(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "input.csv")
	fc, builders := startConnector(t, path, hotReloadCSVBase)

	// Fetch the first page (1 of 2 users) so a token is in flight.
	firstPage, results, err := builders["user"].List(ctx, nil,
		rs.SyncOpAttrs{PageToken: pagination.Token{Size: 1}})
	require.NoError(t, err)
	require.Len(t, firstPage, 1)
	require.NotEmpty(t, results.NextPageToken)

	// The file changes and a Validate (sync start or health probe) swaps the
	// cache while the listing is in flight.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVUpdated), 0o600))
	_, err = fc.Validate(ctx)
	require.NoError(t, err)

	// Resuming with the stale token must restart the listing against the new
	// generation — every current user is returned exactly once, rather than
	// offsets silently skipping or duplicating rows across generations.
	seen := map[string]int{}
	token := results.NextPageToken
	for {
		page, res, err := builders["user"].List(ctx, nil,
			rs.SyncOpAttrs{PageToken: pagination.Token{Token: token, Size: 1}})
		require.NoError(t, err)
		for _, r := range page {
			seen[r.GetId().GetResource()]++
		}
		if token = res.NextPageToken; token == "" {
			break
		}
	}
	require.Equal(t, map[string]int{"alex.taylor": 1, "sam.johnson": 1, "jordan.lee": 1}, seen)
}

func TestHotReload_UnchangedFileSkipsRebuildAndSwap(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "input.csv")
	fc, _ := startConnector(t, path, hotReloadCSVBase)

	// A Validate against unchanged content (every health probe, and every
	// sync where nobody edited the file) must return the already-published
	// cache: no rebuild, and critically no pointer swap under an in-flight
	// sync. Pointer identity is the contract.
	before := fc.cache.load()
	_, err := fc.Validate(ctx)
	require.NoError(t, err)
	require.Same(t, before, fc.cache.load())

	// A real change still swaps.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVUpdated), 0o600))
	_, err = fc.Validate(ctx)
	require.NoError(t, err)
	require.NotSame(t, before, fc.cache.load())
}

func TestHotReload_InvalidFileKeepsLastGoodCache(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "input.csv")
	fc, builders := startConnector(t, path, hotReloadCSVBase)

	require.Len(t, allResources(t, builders["user"], nil), 2)

	// The file is replaced with one that fails validation: the sync must
	// fail loudly while the last-known-good data keeps being served.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVInvalid), 0o600))

	_, err := fc.Validate(ctx)
	require.Error(t, err)

	require.Len(t, allResources(t, builders["user"], nil), 2)
}
