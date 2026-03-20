# JSON / JSONC Input Format

Both `.json` and `.jsonc` file extensions are supported. JSONC files may contain `//` line comments, `/* block comments */`, and trailing commas — these are stripped before parsing.

**Templates:** See `templates/baton-file-jsonc-template.jsonc` (full) and `templates/baton-file-jsonc-quickstart-template.jsonc` (minimal). Required fields are annotated with `// REQUIRED` comments.

## Breaking Changes

- `name` → `id` in `users` and `resources` objects
- `resource_function` → `trait` in `resources` objects
- `entitlement` → `entitlement_slug` and `resource_name` → `resource_id` in `entitlements` objects
- `grants` key removed → replaced by `direct_user_grants` and `grant_inheritance_mappings`
- Using old field names produces a clear error with migration guidance

## Top-Level Structure

```jsonc
{
  "users": [],
  "resources": [],
  "entitlements": [],
  "direct_user_grants": [],
  "grant_inheritance_mappings": []  // optional — only needed for resource-to-resource inheritance
}
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
| `login_aliases` | string or string[] | no | Alternative login identifiers |
| `employee_id` | string or string[] | no | Employee identifier(s) |
| `additional_emails` | string or string[] | no | Non-primary email addresses |
| `mfa_enabled` | boolean | no | Whether MFA is enabled (null/absent = not reported) |
| `sso_enabled` | boolean | no | Whether SSO is enabled (null/absent = not reported) |
| `status_details` | string | no | Human-readable status explanation |
| `profile` | object | no | Key-value profile attributes |

### FlexibleStringList Fields

Fields marked as `string or string[]` accept either a single string or an array:
```json
{"employee_id": "EMP001"}
{"employee_id": ["EMP001", "CONTRACTOR-99"]}
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
| `profile` | object | no | Key-value profile attributes |
| `created_at` | string | no | Secret trait: creation timestamp |
| `expires_at` | string | no | Secret trait: expiration timestamp |
| `created_by` | string | no | Secret trait: creator reference, format `type:id` |
| `identity` | string | no | Secret trait: identity reference, format `type:id` |

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

## Date Formats

Timestamps accept multiple formats including:
- ISO 8601 / RFC 3339: `2025-03-15T10:30:00Z`
- Date only: `2025-03-15`
- US date: `03/15/2025` (slash dates assume MM/DD/YYYY)
- Unix timestamp (seconds): `1710496200`
- Unix timestamp (milliseconds): `1710496200000`

**Note:** Slash-format dates assume US format (MM/DD/YYYY). European users should use ISO 8601 (`YYYY-MM-DD`).
