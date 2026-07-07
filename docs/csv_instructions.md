# CSV Input Format

**Templates:** See `templates/baton-file-csv-template.csv` (full) and `templates/baton-file-csv-quickstart-template.csv` (minimal). CSV does not support comments, so consult the field tables below for which columns are required per record type.

## Breaking Changes

The CSV format is new and uses the updated field/section names from the start. There are no legacy field names to migrate from.

## Overview

CSV files use a single flat file with a `record_type` column to distinguish between different data types. All record types share the same header row — columns that don't apply to a given record type are left empty.

## Required Column

| Column | Description |
|--------|-------------|
| `record_type` | **Required.** One of: `user`, `resource`, `entitlement`, `direct_user_grant`, `grant_inheritance_mapping`, `external_grant` |

## Columns by Record Type

### `user`

| Column | Required | Description |
|--------|----------|-------------|
| `id` | yes | Unique user identifier |
| `display_name` | yes | Display name |
| `email` | no | Primary email address |
| `status` | no | `enabled`, `active`, `disabled`, `inactive`, `suspended`, `deleted` |
| `type` | no | `human` or `service`/`system`/`bot`/`machine` |
| `last_login` | no | Last login timestamp |
| `login` | no | Login username |
| `login_aliases` | no | Comma-separated alternative logins |
| `employee_id` | no | Comma-separated employee ID(s) |
| `additional_emails` | no | Comma-separated non-primary emails |
| `mfa_enabled` | no | `true`/`false`/`1`/`0` (empty = null) |
| `sso_enabled` | no | `true`/`false`/`1`/`0` (empty = null) |
| `status_details` | no | Human-readable status explanation |
| `profile.*` | no | Dynamic profile columns (see below) |

### `resource`

| Column | Required | Description |
|--------|----------|-------------|
| `id` | yes | Unique resource identifier |
| `display_name` | yes | Display name |
| `resource_type` | yes | Resource type string |
| `trait` | yes | `group`, `role`, `app`, `secret`, `user` |
| `description` | no | Description |
| `parent_resource` | no | ID of the parent resource |
| `profile.*` | no | Dynamic profile columns |
| `created_at` | no | Secret trait: creation timestamp |
| `expires_at` | no | Secret trait: expiration timestamp |
| `created_by` | no | Secret trait: creator reference (`type:id`) |
| `identity` | no | Secret trait: identity reference (`type:id`) |

### `entitlement`

| Column | Required | Description |
|--------|----------|-------------|
| `resource_id` | yes | ID of the resource this entitlement is on |
| `entitlement_slug` | yes | Entitlement slug |
| `display_name` | no | Display name |
| `description` | no | Description |

### `direct_user_grant`

| Column | Required | Description |
|--------|----------|-------------|
| `principal_id` | yes | User ID receiving the grant |
| `resource_id` | yes | Resource ID owning the entitlement |
| `entitlement_slug` | yes | Entitlement slug being granted |

### `grant_inheritance_mapping`

| Column | Required | Description |
|--------|----------|-------------|
| `principal_resource_id` | yes | Resource whose membership triggers inheritance |
| `principal_entitlement_slug` | yes | Entitlement slug on the principal resource |
| `inherited_resource_id` | yes | Resource whose entitlement is inherited |
| `inherited_entitlement_slug` | yes | Entitlement slug inherited |
| `inheritance_depth` | yes | `full` or `shallow` |

### `external_grant`

Grants a local entitlement to a principal owned by **another connector** (the
"shared identity source", e.g. Okta or Active Directory). The principal is not
defined in this file — a match rule tells ConductorOne how to find it in the
identity source's synced data. Requires configuring a shared identity source
for this connector in ConductorOne (or `--external-resource-c1z` when running
locally).

| Column | Required | Description |
|--------|----------|-------------|
| `resource_id` | yes | Local resource ID owning the entitlement |
| `entitlement_slug` | yes | Local entitlement slug being granted |
| `match_type` | yes | `all` (every external principal of the type), `id` (match by native ID), or `attribute` (match by key/value) |
| `match_resource_type` | yes | External principal type: `user` or `group` |
| `match_id` | when `match_type` is `id` | The external principal's native ID in the identity source |
| `match_key` | when `match_type` is `attribute` | Attribute to match on (`email` is the standard key for users) |
| `match_value` | when `match_type` is `attribute` | Attribute value to match |
| `expand_entitlement_slug` | no | Grant expansion: members of the matched external group inherit this grant through the group's named entitlement (e.g. `member`). Only valid for `group` matches with `match_type` `id` or `attribute` |
| `expand_depth` | no | `full` (transitive, default) or `shallow` (one level). Only meaningful with `expand_entitlement_slug` |

## Profile Columns

Any column with a header starting with `profile.` is treated as a profile attribute. The prefix is stripped and lowercased to form the key.

Example: A column `profile.department` with value `Engineering` adds `{"department": "Engineering"}` to the record's profile map.

## Comma-Separated List Fields

The fields `employee_id`, `login_aliases`, and `additional_emails` support comma-separated values within a single cell. If a value itself contains a comma, wrap the cell in double quotes per RFC 4180:

```csv
employee_id
"EMP001,CONTRACTOR-99"
```

## Boolean Fields

Boolean columns (`mfa_enabled`, `sso_enabled`) accept:
- `true` or `1` → true
- `false` or `0` → false
- empty → null (not reported)

## Date Formats

Timestamps accept multiple formats including ISO 8601, US dates (MM/DD/YYYY), and Unix timestamps. Slash-format dates assume US format. European users should use ISO 8601 (`YYYY-MM-DD`).

## Quoting Rules

Standard RFC 4180 CSV. Use double quotes around values that contain commas, newlines, or double quotes. Escape double quotes by doubling them (`""`).
