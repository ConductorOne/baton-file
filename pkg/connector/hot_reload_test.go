package connector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Hot-load contract tests (see cacheHolder in connector.go).
//
// Construction goes through the real SDK entry point,
// connectorbuilder.NewConnector, so the SDK itself dictates the lifecycle:
// ResourceSyncers() runs ONCE during construction (the first file load),
// and Validate() runs afterwards, once per sync. The call order is not
// hand-assembled in these tests precisely so that a wrong assumption about
// it cannot produce passing tests. Assertions go through the same server
// RPCs the SDK syncer uses (ListResources, ListGrants, Validate).
//
// Unlike the other connector tests, fixtures here are real files written to
// disk rather than in-memory LoadedData: hot-load IS the
// file → refresh → cache path, so bypassing the loaders would test nothing.

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

// startServer writes the CSV and constructs the connector through the real
// SDK entry point, mirroring a service-mode process start.
func startServer(t *testing.T, path, csv string) (types.ConnectorServer, *FileConnector) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(csv), 0o600))
	fc := &FileConnector{inputFilePath: path}
	server, err := connectorbuilder.NewConnector(context.Background(), fc)
	require.NoError(t, err)
	return server, fc
}

func serverValidate(t *testing.T, server types.ConnectorServer) error {
	t.Helper()
	_, err := server.Validate(context.Background(), v2.ConnectorServiceValidateRequest_builder{}.Build())
	return err
}

// listAll pages through ListResources for one resource type, with pageSize
// controlling how many resources each RPC returns (0 = server default).
func listAll(t *testing.T, server types.ConnectorServer, typeID string, pageSize int) []*v2.Resource {
	t.Helper()
	ctx := context.Background()
	var out []*v2.Resource
	token := ""
	for {
		resp, err := server.ListResources(ctx, v2.ResourcesServiceListResourcesRequest_builder{
			ResourceTypeId: typeID,
			PageToken:      token,
			PageSize:       uint32(pageSize), //nolint:gosec // test-controlled small values
		}.Build())
		require.NoError(t, err)
		out = append(out, resp.GetList()...)
		if token = resp.GetNextPageToken(); token == "" {
			return out
		}
	}
}

func listGrants(t *testing.T, server types.ConnectorServer, res *v2.Resource) []*v2.Grant {
	t.Helper()
	ctx := context.Background()
	var out []*v2.Grant
	token := ""
	for {
		resp, err := server.ListGrants(ctx, v2.GrantsServiceListGrantsRequest_builder{
			Resource:  res,
			PageToken: token,
		}.Build())
		require.NoError(t, err)
		out = append(out, resp.GetList()...)
		if token = resp.GetNextPageToken(); token == "" {
			return out
		}
	}
}

func findResource(t *testing.T, server types.ConnectorServer, typeID, id string) *v2.Resource {
	t.Helper()
	for _, r := range listAll(t, server, typeID, 0) {
		if r.GetId().GetResource() == id {
			return r
		}
	}
	t.Fatalf("resource %s/%s not found", typeID, id)
	return nil
}

func TestHotReload_DataChangesPickedUpBySync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.csv")
	server, _ := startServer(t, path, hotReloadCSVBase)

	require.Len(t, listAll(t, server, "user", 0), 2)
	engineering := findResource(t, server, "team", "engineering")
	require.Len(t, listGrants(t, server, engineering), 1)

	// The customer edits the file while the service keeps running.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVUpdated), 0o600))

	// Validate() is the per-sync hook the SDK runs at the start of every
	// sync; it alone must refresh the data the registered syncers serve.
	require.NoError(t, serverValidate(t, server))

	require.Len(t, listAll(t, server, "user", 0), 3)
	require.Len(t, listGrants(t, server, engineering), 2)
}

func TestHotReload_SwapMidListingRestartsPagination(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "input.csv")
	server, _ := startServer(t, path, hotReloadCSVBase)

	// Fetch the first page (1 of 2 users) so a token is in flight.
	first, err := server.ListResources(ctx, v2.ResourcesServiceListResourcesRequest_builder{
		ResourceTypeId: "user",
		PageSize:       1,
	}.Build())
	require.NoError(t, err)
	require.Len(t, first.GetList(), 1)
	require.NotEmpty(t, first.GetNextPageToken())

	// The file changes and a Validate (sync start or health probe) swaps
	// the cache while the listing is in flight.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVUpdated), 0o600))
	require.NoError(t, serverValidate(t, server))

	// Resuming with the stale token must restart the listing against the
	// new generation — every current user is returned exactly once, rather
	// than offsets silently skipping or duplicating rows across generations.
	seen := map[string]int{}
	token := first.GetNextPageToken()
	for {
		resp, err := server.ListResources(ctx, v2.ResourcesServiceListResourcesRequest_builder{
			ResourceTypeId: "user",
			PageToken:      token,
			PageSize:       1,
		}.Build())
		require.NoError(t, err)
		for _, r := range resp.GetList() {
			seen[r.GetId().GetResource()]++
		}
		if token = resp.GetNextPageToken(); token == "" {
			break
		}
	}
	require.Equal(t, map[string]int{"alex.taylor": 1, "sam.johnson": 1, "jordan.lee": 1}, seen)
}

func TestHotReload_UnchangedFileSkipsRebuildAndSwap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.csv")
	server, fc := startServer(t, path, hotReloadCSVBase)

	// A Validate against unchanged content (every health probe, and every
	// sync where nobody edited the file) must keep serving the cache the
	// construction-time load published: no rebuild, and critically no
	// pointer swap under an in-flight sync. Pointer identity is the
	// contract.
	before := fc.cache.load()
	require.NotNil(t, before)
	require.NoError(t, serverValidate(t, server))
	require.Same(t, before, fc.cache.load())

	// A real change still swaps.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVUpdated), 0o600))
	require.NoError(t, serverValidate(t, server))
	require.NotSame(t, before, fc.cache.load())
}

func TestHotReload_InvalidFileKeepsLastGoodCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.csv")
	server, _ := startServer(t, path, hotReloadCSVBase)

	require.Len(t, listAll(t, server, "user", 0), 2)

	// The file is replaced with one that fails validation: the sync must
	// fail loudly while the last-known-good data keeps being served.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVInvalid), 0o600))
	require.Error(t, serverValidate(t, server))

	require.Len(t, listAll(t, server, "user", 0), 2)
}

func TestHotReload_InvalidFileAtStartupRequiresRestart(t *testing.T) {
	// Pins the known limit documented on cacheHolder: hot-load cannot
	// recover a process that STARTED with an invalid file. ResourceSyncers()
	// runs once at construction; on load failure it registers zero types,
	// and that registration never happens again.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "input.csv")
	server, _ := startServer(t, path, hotReloadCSVInvalid)

	_, err := server.ListResourceTypes(ctx, v2.ResourceTypesServiceListResourceTypesRequest_builder{}.Build())
	require.Equal(t, codes.FailedPrecondition, status.Code(err))

	// The operator fixes the file. Validate now succeeds (and publishes a
	// good cache), but the type registration is gone for the process
	// lifetime — syncs still fail. Only a restart recovers.
	require.NoError(t, os.WriteFile(path, []byte(hotReloadCSVBase), 0o600))
	require.NoError(t, serverValidate(t, server))

	_, err = server.ListResourceTypes(ctx, v2.ResourceTypesServiceListResourceTypesRequest_builder{}.Build())
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
