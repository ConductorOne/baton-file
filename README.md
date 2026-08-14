![Baton Logo](./docs/images/baton-logo.png)

#

`baton-file` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-file.svg)](https://pkg.go.dev/github.com/conductorone/baton-file) ![ci](https://github.com/conductorone/baton-file/actions/workflows/ci.yaml/badge.svg) ![verify](https://github.com/conductorone/baton-file/actions/workflows/verify.yaml/badge.svg)

`baton-file` is a read-only connector that syncs identity security data from YAML, JSON/JSONC, CSV, or Excel (XLSX) files into C1 using the [Baton SDK](https://github.com/conductorone/baton-sdk).

> **BREAKING CHANGES in this version:** Field names, grant structure, and XLSX sheet names
> have changed. Files using the old format will produce clear error messages with migration
> guidance. See the format-specific documentation in `docs/` for details.

## Key Features

- **Multiple file formats:** YAML, JSON, JSONC (JSON with comments), CSV, and XLSX
- **All resource traits:** user, group, role, app, secret
- **Grant inheritance:** Direct user grants and resource-to-resource inheritance mappings with configurable depth
- **Rich user attributes:** MFA/SSO status, employee IDs, login aliases, additional emails, status details
- **Resource profiles:** Key-value metadata on groups, roles, apps
- **Secret trait support:** Created/expires timestamps, creator and identity references
- **Flexible date parsing:** ISO 8601, US dates, Unix timestamps, and many more formats
- **Deprecated field detection:** Old field names produce clear error messages with migration guidance

## Quick Start

```bash
make build
dist/darwin_arm64/baton-file -i templates/baton-file-yaml-quickstart-template.yaml
```

## Usage

```bash
baton-file --input <path-to-file>
baton-file -i data.yaml
baton-file -i data.jsonc
baton-file -i data.json
baton-file -i data.csv
baton-file -i data.xlsx
```

## Hot-Load (File Change Detection)

When baton-file runs as a long-lived service, edits to the input file are picked up automatically at the start of the next sync cycle — no restart required. This applies to the **data** sections: users, resources, entitlements, grants, and inheritance mappings. **Schema changes** — introducing a brand-new resource type, or changing an existing type's trait — still require a service restart, because the SDK registers resource types and their traits once at process startup. If the edited file fails validation, the sync fails with the validation error and the connector keeps serving the last successfully loaded data. If the file changes while a sync is in flight (for example via health-check revalidation), affected listings restart safely against the new contents instead of resuming into stale page offsets; if the same listing restarts more than three times in a row without completing a page because the file keeps changing mid-sync, the sync fails with a "rewritten faster than syncs can read it" error and the next sync starts fresh. A file that is valid but has no data rows is a legitimate empty state: the connector registers its standard resource types (user, group, role, app, secret), syncs empty, and hot-loads data rows as they are added — though rows using custom resource type IDs still require a restart, like any schema change. Hot-load does require a service that started successfully: if the input file is **invalid** when the service first starts, no resource types get registered and syncs fail until the file is fixed **and the service is restarted**.

**IMPORTANT (maintainers and AI agents):** this behavior is a deliberate contract, not an accident of implementation. The SDK calls `ResourceSyncers()` exactly once per process, so `Validate()` — which runs at the start of every sync — is the only per-sync hook and is where the file is re-read and the shared cache is republished (see `cacheHolder` in `pkg/connector/connector.go`). The construction-time load in `ResourceSyncers()` is the *first* load, never the only one: do not capture cache snapshots in builders, and do not assume the SDK refreshes data between syncs (it does not — that assumption caused a regression once already). `TestHotReload_DataChangesPickedUpBySync` in `pkg/connector/hot_reload_test.go` enforces this contract; if it fails after your change, the change is wrong, not the test.

## Templates

Full templates demonstrate every field and feature. Quickstart templates have the minimum to get a working sync (two users, a group, a role, and direct grants).

| Template                                     | Description                                       |
| -------------------------------------------- | ------------------------------------------------- |
| `baton-file-yaml-template.yaml`              | Full YAML with all fields                         |
| `baton-file-jsonc-template.jsonc`            | Full JSONC with all fields                        |
| `baton-file-csv-template.csv`                | Full CSV with all fields                          |
| `baton-file-excel-template.xlsx`             | Full XLSX with Instructions and Quickstart sheets |
| `baton-file-yaml-quickstart-template.yaml`   | Minimal YAML                                      |
| `baton-file-jsonc-quickstart-template.jsonc` | Minimal JSONC                                     |
| `baton-file-csv-quickstart-template.csv`     | Minimal CSV                                       |

Required fields are annotated with `# REQUIRED` (YAML) or `// REQUIRED` (JSONC) comments in the templates. The XLSX template includes an Instructions sheet with a complete field reference and a Quickstart sheet with a step-by-step guide.

## File Formats

All formats share the same data model with five sections:

| Section                      | Description                                        |
| ---------------------------- | -------------------------------------------------- |
| `users`                      | User identity definitions                          |
| `resources`                  | Non-user resources (teams, roles, apps, secrets)   |
| `entitlements`               | Permission/access definitions on resources         |
| `direct_user_grants`         | User-to-entitlement grant assignments              |
| `grant_inheritance_mappings` | Resource-to-resource entitlement inheritance       |
| `external_grants`            | Grants to principals from a shared identity source |

See format-specific documentation:

- [YAML](docs/yaml_instructions.md)
- [JSON / JSONC](docs/json_instructions.md)
- [CSV](docs/csv_instructions.md)
- [Excel (XLSX)](docs/excel_instructions.md)

## Resource Traits

| Trait    | Use Case                                        |
| -------- | ----------------------------------------------- |
| `user`   | Human identities (defined in `users` section)   |
| `group`  | Collections with membership (teams, workspaces) |
| `role`   | Permission sets                                 |
| `app`    | Applications, service accounts                  |
| `secret` | API keys, tokens, credentials                   |

## Grant Types

### Direct User Grants

Assign an entitlement directly to a user:

```yaml
direct_user_grants:
  - principal_id: jane.smith
    resource_id: engineering
    entitlement_slug: member
```

### Grant Inheritance Mappings

Define that members of one resource inherit entitlements on another:

```yaml
grant_inheritance_mappings:
  - principal_resource_id: engineering
    principal_entitlement_slug: member
    inherited_resource_id: api-service
    inherited_entitlement_slug: read
    inheritance_depth: full # or "shallow"
```

### External Grants (Shared Identity Source)

Assign a local entitlement to a principal owned by **another connector** (the
"shared identity source", e.g. Okta or Active Directory). The principal is not
defined in this file — a match rule tells ConductorOne how to find it in the
identity source's synced data:

```yaml
external_grants:
  # Match one external user by attribute (email is the standard key for users)
  - resource_id: api-service
    entitlement_slug: write
    match_type: attribute # all, id, or attribute
    match_resource_type: user # user or group
    match_key: email
    match_value: jane.smith@external.com

  # Match one external principal by its native ID in the identity source.
  # For group matches, expand_entitlement_slug optionally enables grant
  # expansion: members of the matched group inherit this grant through the
  # group's named entitlement. expand_depth is "full" (default) or "shallow".
  - resource_id: api-service
    entitlement_slug: read
    match_type: id
    match_resource_type: group
    match_id: external-group-123
    expand_entitlement_slug: member

  # Match ALL external principals of a type
  - resource_id: api-service
    entitlement_slug: read
    match_type: all
    match_resource_type: user
```

To use external grants, configure a shared identity source for this connector
in ConductorOne. When running locally, pass the identity source's synced c1z
via `--external-resource-c1z` (and optionally
`--external-resource-entitlement-id-filter` to limit which external principals
are considered).

## Date Handling

Timestamps accept multiple formats including ISO 8601, RFC 3339, US dates (MM/DD/YYYY), and Unix timestamps (seconds or milliseconds).

**Note:** Slash-format dates (e.g., `03/15/2025`) assume US format (MM/DD/YYYY). European users should use ISO 8601 (`YYYY-MM-DD`).

```
baton-file

Usage:
  baton-file [flags]
  baton-file [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  help               Help about any command

Flags:
  -i, --input string                             Path to the input file.
      --client-id string                         The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string                     The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
      --disable-audit-log-feed                   If checked, disables the resource changed events feed ($BATON_DISABLE_AUDIT_LOG_FEED)
  -f, --file string                              The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                                     help for baton-file
      --log-format string                        The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string                         The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -v, --version                                  version for baton-file

Use "baton-file [command] --help" for more information about a command.
```

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually building spreadsheets. We welcome contributions, and ideas, no matter how small -- our goal is to make identity and permissions sprawl less painful for everyone. If you have questions, problems, or ideas: Please open a Github Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.
