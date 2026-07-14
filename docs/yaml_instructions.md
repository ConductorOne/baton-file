# YAML Input Format

**Templates:** See `templates/baton-file-yaml-template.yaml` (full) and `templates/baton-file-yaml-quickstart-template.yaml` (minimal). Required fields are annotated with `# REQUIRED` comments.

## Breaking Changes

- `name` → `id` in `users` and `resources` sections
- `resource_function` → `trait` in `resources` section
- `entitlement` → `entitlement_slug` and `resource_name` → `resource_id` in `entitlements` section
- `grants` section removed → replaced by `direct_user_grants` and `grant_inheritance_mappings`
- Using old field names produces a clear error with migration guidance

## Top-Level Structure

```yaml
users: []
resources: []
entitlements: []
direct_user_grants: []
grant_inheritance_mappings: []
external_grants: []
```

## Users

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique user identifier |
| `display_name` | string | yes | Display name |
| `email` | string | no | Primary email address |
| `status` | string | no | `enabled`, `active`, `disabled`, `inactive`, `suspended`, `deleted` (default: enabled) |
| `type` | string | no | `human` or `service`/`system`/`bot`/`machine` (default: human) |
| `last_login` | string | no | Last login timestamp (multi-format, see Date Formats) |
| `login` | string | no | Login username |
| `login_aliases` | string or string[] | no | Alternative login identifiers (FlexibleStringList) |
| `employee_id` | string or string[] | no | Employee identifier(s) (FlexibleStringList) |
| `additional_emails` | string or string[] | no | Non-primary email addresses (FlexibleStringList) |
| `mfa_enabled` | boolean | no | Whether MFA is enabled (null = not reported) |
| `sso_enabled` | boolean | no | Whether SSO is enabled (null = not reported) |
| `status_details` | string | no | Human-readable status explanation |
| `profile` | map | no | Key-value profile attributes |

### FlexibleStringList Fields

Fields marked as `string or string[]` accept either format:
```yaml
employee_id: "EMP001"           # single string
employee_id:                     # list
  - "EMP001"
  - "CONTRACTOR-99"
```

## Resources

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resource_type` | string | yes | Resource type (e.g., `team`, `role`, `application`) |
| `trait` | string | yes | Trait: `group`, `role`, `app`, `secret`, `user` |
| `id` | string | yes | Unique resource identifier |
| `display_name` | string | yes | Display name |
| `description` | string | no | Description |
| `parent_resource` | string | no | ID of the parent resource |
| `profile` | map | no | Key-value profile attributes |
| `created_at` | string | no | Secret trait: creation timestamp |
| `expires_at` | string | no | Secret trait: expiration timestamp |
| `created_by` | string | no | Secret trait: creator reference, format `type:id` (e.g., `user:alice`) |
| `identity` | string | no | Secret trait: identity reference, format `type:id` (e.g., `application:billing_app`) |

Secret-specific fields (`created_at`, `expires_at`, `created_by`, `identity`) are only applicable when `trait` is `secret`. They are ignored with a warning on other traits.

## Entitlements

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resource_id` | string | yes | ID of the resource this entitlement is defined on |
| `entitlement_slug` | string | yes | Entitlement slug (unique per resource) |
| `display_name` | string | no | Display name |
| `description` | string | no | Description |

## Direct User Grants

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `principal_id` | string | yes | User ID receiving the grant |
| `resource_id` | string | yes | Resource ID owning the entitlement |
| `entitlement_slug` | string | yes | Entitlement slug being granted |

## Grant Inheritance Mappings

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `principal_resource_id` | string | yes | Resource whose membership triggers inheritance |
| `principal_entitlement_slug` | string | yes | Entitlement slug on the principal resource |
| `inherited_resource_id` | string | yes | Resource whose entitlement is inherited |
| `inherited_entitlement_slug` | string | yes | Entitlement slug that is inherited |
| `inheritance_depth` | string | yes | Must be `"full"` or `"shallow"` |

## External Grants

Grants a local entitlement to a principal owned by **another connector** (the
"shared identity source", e.g. Okta or Active Directory). The principal is not
defined in this file — a match rule tells ConductorOne how to find it in the
identity source's synced data. Requires configuring a shared identity source
for this connector in ConductorOne (or `--external-resource-c1z` when running
locally).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resource_id` | string | yes | Local resource ID owning the entitlement |
| `entitlement_slug` | string | yes | Local entitlement slug being granted |
| `match_type` | string | yes | `all` (every external principal of the type), `id` (match by native ID), or `attribute` (match by key/value) |
| `match_resource_type` | string | yes | External principal type: `user` or `group` |
| `match_id` | string | when `match_type: id` | The external principal's native ID in the identity source |
| `match_key` | string | when `match_type: attribute` | Attribute to match on (`email` is the standard key for users) |
| `match_value` | string | when `match_type: attribute` | Attribute value to match |
| `expand_entitlement_slug` | string | no | Grant expansion: members of the matched external group inherit this grant through the group's named entitlement (e.g. `member`). Only valid for `group` matches with `match_type` `id` or `attribute` |
| `expand_depth` | string | no | `full` (transitive, default) or `shallow` (one level). Only meaningful with `expand_entitlement_slug` |

## Date Formats

Timestamps accept multiple formats including:
- ISO 8601 / RFC 3339: `2025-03-15T10:30:00Z`
- Date only: `2025-03-15`
- US date: `03/15/2025` (slash dates assume MM/DD/YYYY)
- Unix timestamp (seconds): `1710496200`
- Unix timestamp (milliseconds): `1710496200000`

**Note:** Slash-format dates assume US format (MM/DD/YYYY). European users should use ISO 8601 (`YYYY-MM-DD`).

## Supported Traits

| Trait | SDK Enum | Typical Use |
|-------|----------|-------------|
| `user` | `TRAIT_USER` | Human identities |
| `group` | `TRAIT_GROUP` | Collections with membership |
| `role` | `TRAIT_ROLE` | Permission sets |
| `app` | `TRAIT_APP` | Service accounts, API keys |
| `secret` | `TRAIT_SECRET` | Secrets, API keys, tokens |
