# Excel (XLSX) Input Format

**Templates:** The XLSX template ships with an `Instructions` sheet (full field reference) and a `Quickstart` sheet (step-by-step minimal setup). Required columns are noted in both sheets. See the `z/xlsx/` directory for per-sheet CSVs to assemble the workbook.

## Breaking Changes

- Sheet `Grants` is no longer supported → replaced by `Direct User Grants` and `Grant Inheritance Mappings` sheets
- Users sheet: `Name` column → `ID`
- Resources sheet: `Name` → `ID`, `Resource Function` → `Trait`
- Entitlements sheet: `Resource Name` → `Resource ID`, `Entitlement` → `Entitlement Slug`
- Old column/sheet names produce clear error messages with migration guidance

## Sheet Names

Sheet names use Title Case with spaces. Lowercase names are also accepted for backward compatibility with the three original sheets.

| Sheet | Description |
|-------|-------------|
| `Instructions` | Read-only instructions (ignored by the connector) |
| `Users` | User definitions |
| `Resources` | Non-user resource definitions |
| `Entitlements` | Entitlement definitions |
| `Direct User Grants` | Direct user-to-entitlement grants |
| `Grant Inheritance Mappings` | Grant inheritance between resources |

## Users Sheet

| Column | Required | Description |
|--------|----------|-------------|
| `ID` | yes | Unique user identifier |
| `Display Name` | yes | Display name |
| `Email` | no | Primary email address |
| `Status` | no | `enabled`, `active`, `disabled`, `inactive`, `suspended`, `deleted` |
| `Type` | no | `human` or `service`/`system`/`bot`/`machine` |
| `Last Login` | no | Last login timestamp (multi-format) |
| `Login` | no | Login username |
| `Login Aliases` | no | Comma-separated alternative login identifiers |
| `Employee ID` | no | Comma-separated employee identifier(s) |
| `Additional Emails` | no | Comma-separated non-primary email addresses |
| `MFA Enabled` | no | `true`/`false` |
| `SSO Enabled` | no | `true`/`false` |
| `Status Details` | no | Human-readable status explanation |
| `Profile: {key}` | no | Dynamic profile columns (e.g., `Profile: department`) |

## Resources Sheet

| Column | Required | Description |
|--------|----------|-------------|
| `Resource Type` | yes | Resource type string |
| `Trait` | yes | `group`, `role`, `app`, `secret`, `user` |
| `ID` | yes | Unique resource identifier |
| `Display Name` | yes | Display name |
| `Description` | no | Description |
| `Parent Resource` | no | ID of the parent resource |
| `Profile: {key}` | no | Dynamic profile columns (e.g., `Profile: region`) |
| `Created At` | no | Secret trait: creation timestamp |
| `Expires At` | no | Secret trait: expiration timestamp |
| `Created By` | no | Secret trait: creator reference (`type:id`) |
| `Identity` | no | Secret trait: identity reference (`type:id`) |

### Profile Columns

Profile columns use the format `Profile: key_name` as the header. The key is lowercased and stripped of the prefix. For example, a column named `Profile: slack_channel` adds `{"slack_channel": "value"}` to the profile map.

## Entitlements Sheet

| Column | Required | Description |
|--------|----------|-------------|
| `Resource ID` | yes | ID of the resource this entitlement is on |
| `Entitlement Slug` | yes | Entitlement slug |
| `Display Name` | no | Display name |
| `Description` | no | Description |

## Direct User Grants Sheet

| Column | Required | Description |
|--------|----------|-------------|
| `Principal ID` | yes | User ID receiving the grant |
| `Resource ID` | yes | Resource ID owning the entitlement |
| `Entitlement Slug` | yes | Entitlement slug being granted |

## Grant Inheritance Mappings Sheet

| Column | Required | Description |
|--------|----------|-------------|
| `Principal Resource ID` | yes | Resource whose membership triggers inheritance |
| `Principal Entitlement Slug` | yes | Entitlement slug on the principal resource |
| `Inherited Resource ID` | yes | Resource whose entitlement is inherited |
| `Inherited Entitlement Slug` | yes | Entitlement slug inherited |
| `Inheritance Depth` | yes | `full` or `shallow` |

## Date Formats

Timestamps accept multiple formats including ISO 8601, US dates (MM/DD/YYYY), and Unix timestamps. Slash-format dates assume US format. European users should use ISO 8601 (`YYYY-MM-DD`).
