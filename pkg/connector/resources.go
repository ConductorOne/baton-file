package connector

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/conductorone/baton-file/pkg/client"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

const pageSize = 1000

// clampPageSize returns requested if it is within (0, pageSize], otherwise
// returns pageSize. Guards against zero (SDK default) and oversized requests.
func clampPageSize(requested int) int {
	if requested <= 0 || requested > pageSize {
		return pageSize
	}
	return requested
}

// parseOffset validates a numeric page-token offset.
func parseOffset(tokenStr, offsetStr string) (int, error) {
	parsed, err := strconv.Atoi(offsetStr)
	if err != nil {
		return 0, fmt.Errorf("baton-file: invalid page token %q: %w", tokenStr, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("baton-file: invalid page token %q: must be a positive integer", tokenStr)
	}
	return parsed, nil
}

func paginate[T any](items []T, gen string, tokenStr string, tokenSize int) ([]T, string, error) {
	// SDK sends Size=0 during sync_full; clamp converts it to pageSize so we never return an empty page.
	size := clampPageSize(tokenSize)

	// Empty token means first page. Tokens minted by this version are
	// "<gen>:<offset>" where gen fingerprints the cache the offset indexes
	// into; bare numeric tokens are the legacy (pre-generation) format.
	offset := 0
	if tokenStr != "" {
		tokenGen, offsetStr, found := strings.Cut(tokenStr, ":")
		switch {
		case !found:
			// Bare numeric token. If this cache has a generation (every
			// production cache does — loadValidatedCache always stamps one),
			// the token is legacy, minted by a pre-generation binary. Its
			// generation is unknowable and it can only arrive after a
			// process restart — exactly when the file may have changed while
			// the service was down — so restart the listing like a
			// generation mismatch rather than replay an offset into a slice
			// it was never minted against. A generation-less cache (tests
			// build them directly) mints bare tokens itself, so there the
			// offset is trusted; restarting would loop forever. Malformed
			// tokens are rejected either way.
			parsed, err := parseOffset(tokenStr, tokenStr)
			if err != nil {
				return nil, "", err
			}
			if gen == "" {
				offset = parsed
			}
		case tokenGen != gen:
			// The token was minted against a different cache generation: the
			// file changed and the cache was swapped while this listing was
			// in flight (mid-sync health-check revalidation, or a sync
			// resumed after a restart). Offsets are meaningless across
			// generations — resuming would silently skip or duplicate items.
			// Restart the listing instead: store writes are idempotent
			// upserts, so re-emitting earlier pages is safe.
			offset = 0
		default:
			parsed, err := parseOffset(tokenStr, offsetStr)
			if err != nil {
				return nil, "", err
			}
			offset = parsed
		}
	}

	// Guard against a stale checkpoint: if the file was replaced with fewer
	// items after an interrupted sync, offset may exceed the new slice length.
	// Clamping here prevents items[offset:end] from panicking.
	if offset > len(items) {
		offset = len(items)
	}

	// Clamp end to the slice length so the last page returns only what's left.
	end := offset + size
	if end > len(items) {
		end = len(items)
	}

	// Empty next token signals the SDK that there are no more pages. An empty
	// gen (caches built directly in tests) mints legacy numeric tokens.
	var next string
	if end < len(items) {
		if gen == "" {
			next = strconv.Itoa(end)
		} else {
			next = gen + ":" + strconv.Itoa(end)
		}
	}
	return items[offset:end], next, nil
}

type resourceBuilder struct {
	// cache is the shared live holder, NOT a snapshot. Builders live for the
	// whole process while Validate() republishes the cache each sync.
	// IMPORTANT: do not replace this with a *syncCache captured at
	// construction — that freezes file data until the service restarts
	// (hot-load regression; see cacheHolder in connector.go).
	//
	// It is nil for builders made by StaticCapabilitiesConnector: the
	// capabilities sub-command registers resource types without reading any
	// file. No SDK path lists through those builders, but the sync methods
	// below still guard the nil and return an error instead of panicking.
	cache        *cacheHolder
	resourceType *v2.ResourceType
}

func (b *resourceBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return b.resourceType
}

func (b *resourceBuilder) List(_ context.Context, parentResourceID *v2.ResourceId,
	opts rs.SyncOpAttrs) ([]*v2.Resource, *rs.SyncOpResults, error) {
	cache := b.cache.load()
	if cache == nil {
		return nil, nil, fmt.Errorf("baton-file: sync cache not initialized")
	}
	resources := cache.listIndex[listKey(b.resourceType.GetId(), parentResourceID)]
	page, next, err := paginate(resources, cache.gen, opts.PageToken.Token, opts.PageToken.Size)
	if err != nil {
		return nil, nil, err
	}
	return page, &rs.SyncOpResults{NextPageToken: next}, nil
}

func (b *resourceBuilder) Entitlements(_ context.Context, resource *v2.Resource,
	opts rs.SyncOpAttrs) ([]*v2.Entitlement, *rs.SyncOpResults, error) {
	cache := b.cache.load()
	if cache == nil {
		return nil, nil, fmt.Errorf("baton-file: sync cache not initialized")
	}
	ents := cache.entIndex[resource.GetId().GetResource()]
	page, next, err := paginate(ents, cache.gen, opts.PageToken.Token, opts.PageToken.Size)
	if err != nil {
		return nil, nil, err
	}
	return page, &rs.SyncOpResults{NextPageToken: next}, nil
}

func (b *resourceBuilder) Grants(_ context.Context, resource *v2.Resource,
	opts rs.SyncOpAttrs) ([]*v2.Grant, *rs.SyncOpResults, error) {
	cache := b.cache.load()
	if cache == nil {
		return nil, nil, fmt.Errorf("baton-file: sync cache not initialized")
	}
	grants := cache.grantsIndex[resource.GetId().GetResource()]
	page, next, err := paginate(grants, cache.gen, opts.PageToken.Token, opts.PageToken.Size)
	if err != nil {
		return nil, nil, err
	}
	return page, &rs.SyncOpResults{NextPageToken: next}, nil
}

// The deprecated trait options used below (WithUserProfile, WithStatus,
// WithGroupProfile, WithSecretCreatedAt, ...) are kept INTENTIONALLY. The SDK
// is migrating profile/status/created_at from the trait protos to
// resource-level attributes: the deprecated options populate BOTH levels,
// while the WithResource* replacements populate only the resource level.
// A lint-driven swap would silently drop the trait-level fields from sync
// output — breaking anything downstream that still reads traits — so the
// migration needs its own coordinated change, not a mechanical rename.
func buildUserResource(ctx context.Context, userData client.UserData,
	resourceType *v2.ResourceType) (*v2.Resource, error) {
	l := ctxzap.Extract(ctx)
	var opts []rs.UserTraitOption

	if userData.Email != "" {
		opts = append(opts, rs.WithEmail(userData.Email, true))
	}
	if len(userData.Profile) > 0 {
		opts = append(opts, rs.WithUserProfile(userData.Profile)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
	}

	userStatus := v2.UserTrait_Status_STATUS_ENABLED
	switch strings.ToLower(userData.Status) {
	case "enabled", "active", "":
		// keep default (STATUS_ENABLED)
	case "disabled", "inactive", "suspended":
		userStatus = v2.UserTrait_Status_STATUS_DISABLED
	case "deleted":
		userStatus = v2.UserTrait_Status_STATUS_DELETED
	default:
		l.Warn("baton-file: unknown user status, defaulting to enabled",
			zap.String("user_id", userData.ID), zap.String("status", userData.Status))
	}

	userAccountType := v2.UserTrait_ACCOUNT_TYPE_HUMAN
	switch strings.ToLower(userData.Type) {
	case "human", "":
		// keep default (ACCOUNT_TYPE_HUMAN)
	case "service", "system", "bot", "machine":
		userAccountType = v2.UserTrait_ACCOUNT_TYPE_SERVICE
	default:
		l.Warn("baton-file: unknown user type, defaulting to human",
			zap.String("user_id", userData.ID), zap.String("type", userData.Type))
	}
	opts = append(opts, rs.WithAccountType(userAccountType))

	if userData.LastLogin != "" {
		t, err := client.ParseTime(userData.LastLogin)
		if err != nil {
			l.Warn("baton-file: failed to parse last_login, skipping field",
				zap.String("user_id", userData.ID), zap.Error(err))
		} else {
			opts = append(opts, rs.WithLastLogin(*t))
		}
	}

	if len(userData.EmployeeID) > 0 {
		opts = append(opts, rs.WithEmployeeID(userData.EmployeeID...))
	}

	if userData.Login != "" {
		opts = append(opts, rs.WithUserLogin(userData.Login, userData.LoginAliases...))
	}

	for _, addr := range userData.AdditionalEmails {
		if addr != "" {
			opts = append(opts, rs.WithEmail(addr, false))
		}
	}

	if userData.MFAEnabled != nil {
		opts = append(opts, rs.WithMFAStatus(
			v2.UserTrait_MFAStatus_builder{MfaEnabled: *userData.MFAEnabled}.Build(),
		))
	}

	if userData.SSOEnabled != nil {
		opts = append(opts, rs.WithSSOStatus(
			v2.UserTrait_SSOStatus_builder{SsoEnabled: *userData.SSOEnabled}.Build(),
		))
	}

	if userData.StatusDetails != "" {
		opts = append(opts, rs.WithDetailedStatus(userStatus, userData.StatusDetails)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
	} else {
		opts = append(opts, rs.WithStatus(userStatus)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
	}

	return rs.NewUserResource(userData.DisplayName, resourceType, userData.ID, opts)
}

func buildResource(ctx context.Context, data client.ResourceData,
	resourceType *v2.ResourceType) (*v2.Resource, error) {
	var resourceOpts []rs.ResourceOption

	if data.Description != "" {
		resourceOpts = append(resourceOpts, rs.WithDescription(data.Description))
	}

	if len(resourceType.GetTraits()) > 0 {
		switch resourceType.GetTraits()[0] { //nolint:exhaustive // only user-defined traits are relevant here
		case v2.ResourceType_TRAIT_GROUP:
			resourceOpts = append(resourceOpts, rs.WithGroupTrait(buildGroupTraitOptions(data)...))
		case v2.ResourceType_TRAIT_ROLE:
			resourceOpts = append(resourceOpts, rs.WithRoleTrait(buildRoleTraitOptions(data)...))
		case v2.ResourceType_TRAIT_APP:
			resourceOpts = append(resourceOpts, rs.WithAppTrait(buildAppTraitOptions(data)...))
		case v2.ResourceType_TRAIT_SECRET:
			resourceOpts = append(resourceOpts, rs.WithSecretTrait(buildSecretTraitOptions(ctx, data)...))
		case v2.ResourceType_TRAIT_USER:
			resourceOpts = append(resourceOpts, rs.WithUserTrait())
		}
	}

	return rs.NewResource(data.DisplayName, resourceType, data.ID, resourceOpts...)
}

func buildGroupTraitOptions(data client.ResourceData) []rs.GroupTraitOption {
	var opts []rs.GroupTraitOption
	if len(data.Profile) > 0 {
		opts = append(opts, rs.WithGroupProfile(data.Profile)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
	}
	return opts
}

func buildRoleTraitOptions(data client.ResourceData) []rs.RoleTraitOption {
	var opts []rs.RoleTraitOption
	if len(data.Profile) > 0 {
		opts = append(opts, rs.WithRoleProfile(data.Profile)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
	}
	return opts
}

func buildAppTraitOptions(data client.ResourceData) []rs.AppTraitOption {
	var opts []rs.AppTraitOption
	if len(data.Profile) > 0 {
		opts = append(opts, rs.WithAppProfile(data.Profile)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
	}
	return opts
}

func buildSecretTraitOptions(ctx context.Context, data client.ResourceData) []rs.SecretTraitOption {
	l := ctxzap.Extract(ctx)
	var opts []rs.SecretTraitOption

	if data.CreatedAt != "" {
		t, err := client.ParseTime(data.CreatedAt)
		if err != nil {
			l.Warn("baton-file: failed to parse created_at for secret, skipping",
				zap.String("resource_id", data.ID), zap.Error(err))
		} else {
			opts = append(opts, rs.WithSecretCreatedAt(*t)) //nolint:staticcheck // kept: sets trait+resource field; replacement drops the trait field, changing sync output
		}
	}

	if data.ExpiresAt != "" {
		t, err := client.ParseTime(data.ExpiresAt)
		if err != nil {
			l.Warn("baton-file: failed to parse expires_at for secret, skipping",
				zap.String("resource_id", data.ID), zap.Error(err))
		} else {
			opts = append(opts, rs.WithSecretExpiresAt(*t))
		}
	}

	if data.CreatedBy != "" {
		typePart, idPart, err := client.ParseTypeColonID(data.CreatedBy)
		if err != nil {
			l.Warn("baton-file: invalid created_by format",
				zap.String("resource_id", data.ID), zap.String("created_by", data.CreatedBy), zap.Error(err))
		} else {
			opts = append(opts, rs.WithSecretCreatedByID(
				v2.ResourceId_builder{ResourceType: typePart, Resource: idPart}.Build(),
			))
		}
	}

	if data.Identity != "" {
		typePart, idPart, err := client.ParseTypeColonID(data.Identity)
		if err != nil {
			l.Warn("baton-file: invalid identity format",
				zap.String("resource_id", data.ID), zap.String("identity", data.Identity), zap.Error(err))
		} else {
			opts = append(opts, rs.WithSecretIdentityID(
				v2.ResourceId_builder{ResourceType: typePart, Resource: idPart}.Build(),
			))
		}
	}

	return opts
}
