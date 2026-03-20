package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-file/pkg/client"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

type resourceBuilder struct {
	cache        *syncCache
	resourceType *v2.ResourceType
}

func (b *resourceBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return b.resourceType
}

func (b *resourceBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId,
	opts rs.SyncOpAttrs) ([]*v2.Resource, *rs.SyncOpResults, error) {
	// Full scan is fine — resource types are single-digit and total resources
	// are at most low thousands for a file-based connector.
	var rv []*v2.Resource
	for _, res := range b.cache.resources {
		if res.GetId().GetResourceType() != b.resourceType.GetId() {
			continue
		}
		if parentResourceID != nil {
			if res.GetParentResourceId() == nil ||
				res.GetParentResourceId().GetResourceType() != parentResourceID.GetResourceType() ||
				res.GetParentResourceId().GetResource() != parentResourceID.GetResource() {
				continue
			}
		} else if res.GetParentResourceId() != nil {
			continue
		}
		rv = append(rv, res)
	}
	sort.SliceStable(rv, func(i, j int) bool {
		return rv[i].GetId().GetResource() < rv[j].GetId().GetResource()
	})
	return rv, &rs.SyncOpResults{}, nil
}

// Same full-scan rationale as List() — see comment above.
func (b *resourceBuilder) Entitlements(ctx context.Context, resource *v2.Resource,
	opts rs.SyncOpAttrs) ([]*v2.Entitlement, *rs.SyncOpResults, error) {
	var rv []*v2.Entitlement
	for _, ent := range b.cache.entitlements {
		if ent.GetResource().GetId().GetResourceType() == resource.GetId().GetResourceType() &&
			ent.GetResource().GetId().GetResource() == resource.GetId().GetResource() {
			rv = append(rv, ent)
		}
	}
	sort.SliceStable(rv, func(i, j int) bool {
		return rv[i].GetSlug() < rv[j].GetSlug()
	})
	return rv, &rs.SyncOpResults{}, nil
}

// Same full-scan rationale as List() — see comment above.
func (b *resourceBuilder) Grants(ctx context.Context, resource *v2.Resource,
	opts rs.SyncOpAttrs) ([]*v2.Grant, *rs.SyncOpResults, error) {
	l := ctxzap.Extract(ctx)
	var allGrants []*v2.Grant

	for i, g := range b.cache.loadedData.DirectUserGrants {
		if g.ResourceID != resource.GetId().GetResource() {
			continue
		}

		if _, ok := b.cache.resources[g.PrincipalID]; !ok {
			l.Warn("baton-file: skipping direct grant, principal not found in users",
				zap.String("principal_id", g.PrincipalID), zap.Int("index", i))
			continue
		}

		entKey := fmt.Sprintf("%s:%s", g.ResourceID, g.EntitlementSlug)
		if _, ok := b.cache.entitlements[entKey]; !ok {
			l.Warn("baton-file: skipping direct grant, entitlement not found",
				zap.String("entitlement_key", entKey), zap.Int("index", i))
			continue
		}

		principalId := v2.ResourceId_builder{
			ResourceType: userResourceType.GetId(),
			Resource:     g.PrincipalID,
		}.Build()

		newGrant := grant.NewGrant(resource, g.EntitlementSlug, principalId)
		allGrants = append(allGrants, newGrant)
	}

	for i, m := range b.cache.loadedData.GrantInheritanceMappings {
		if m.InheritedResourceID != resource.GetId().GetResource() {
			continue
		}

		if m.InheritanceDepth != "full" && m.InheritanceDepth != "shallow" {
			l.Error("baton-file: invalid inheritance_depth, must be \"full\" or \"shallow\"",
				zap.String("value", m.InheritanceDepth), zap.Int("index", i))
			continue
		}

		principalResource, ok := b.cache.resources[m.PrincipalResourceID]
		if !ok {
			l.Warn("baton-file: skipping inheritance mapping, principal resource not found",
				zap.String("principal_resource_id", m.PrincipalResourceID), zap.Int("index", i))
			continue
		}

		membershipKey := fmt.Sprintf("%s:%s", m.PrincipalResourceID, m.PrincipalEntitlementSlug)
		membershipEntitlement, ok := b.cache.entitlements[membershipKey]
		if !ok {
			l.Warn("baton-file: skipping inheritance mapping, membership entitlement not found",
				zap.String("membership_key", membershipKey), zap.Int("index", i))
			continue
		}

		expandable := v2.GrantExpandable_builder{
			EntitlementIds: []string{membershipEntitlement.GetId()},
			Shallow:        m.InheritanceDepth == "shallow",
		}.Build()

		newGrant := grant.NewGrant(
			resource,
			m.InheritedEntitlementSlug,
			principalResource.GetId(),
			grant.WithAnnotation(expandable),
		)
		allGrants = append(allGrants, newGrant)
	}

	// Sort by concatenated resource type/id rather than proto String() to
	// ensure deterministic output across protobuf library versions.
	sort.SliceStable(allGrants, func(i, j int) bool {
		pi := allGrants[i].GetPrincipal().GetId()
		pj := allGrants[j].GetPrincipal().GetId()
		ki := pi.GetResourceType() + "/" + pi.GetResource()
		kj := pj.GetResourceType() + "/" + pj.GetResource()
		if ki != kj {
			return ki < kj
		}
		return allGrants[i].GetEntitlement().GetId() < allGrants[j].GetEntitlement().GetId()
	})

	return allGrants, &rs.SyncOpResults{}, nil
}

func buildUserResource(ctx context.Context, userData client.UserData,
	resourceType *v2.ResourceType) (*v2.Resource, error) {
	l := ctxzap.Extract(ctx)
	var opts []rs.UserTraitOption

	if userData.Email != "" {
		opts = append(opts, rs.WithEmail(userData.Email, true))
	}
	if len(userData.Profile) > 0 {
		opts = append(opts, rs.WithUserProfile(userData.Profile))
	}

	userStatus := v2.UserTrait_Status_STATUS_ENABLED
	switch strings.ToLower(userData.Status) {
	case "enabled", "active":
		userStatus = v2.UserTrait_Status_STATUS_ENABLED
	case "disabled", "inactive", "suspended":
		userStatus = v2.UserTrait_Status_STATUS_DISABLED
	case "deleted":
		userStatus = v2.UserTrait_Status_STATUS_DELETED
	case "":
		// keep default
	default:
		l.Warn("baton-file: unknown user status, defaulting to enabled",
			zap.String("user_id", userData.ID), zap.String("status", userData.Status))
	}

	userAccountType := v2.UserTrait_ACCOUNT_TYPE_HUMAN
	switch strings.ToLower(userData.Type) {
	case "human", "":
		userAccountType = v2.UserTrait_ACCOUNT_TYPE_HUMAN
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
		opts = append(opts, rs.WithDetailedStatus(userStatus, userData.StatusDetails))
	} else {
		opts = append(opts, rs.WithStatus(userStatus))
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
		opts = append(opts, rs.WithGroupProfile(data.Profile))
	}
	return opts
}

func buildRoleTraitOptions(data client.ResourceData) []rs.RoleTraitOption {
	var opts []rs.RoleTraitOption
	if len(data.Profile) > 0 {
		opts = append(opts, rs.WithRoleProfile(data.Profile))
	}
	return opts
}

func buildAppTraitOptions(data client.ResourceData) []rs.AppTraitOption {
	var opts []rs.AppTraitOption
	if len(data.Profile) > 0 {
		opts = append(opts, rs.WithAppProfile(data.Profile))
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
			opts = append(opts, rs.WithSecretCreatedAt(*t))
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
