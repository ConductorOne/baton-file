![Baton Logo](./baton-logo.png)

# `baton-file` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-file.svg)](https://pkg.go.dev/github.com/conductorone/baton-file) ![main ci](https://github.com/conductorone/baton-file/actions/workflows/main.yaml/badge.svg)

`baton-file` is a connector built using the [Baton SDK](https://github.com/conductorone/baton-sdk).

Check out [Baton](https://github.com/conductorone/baton) to learn more the project in general.

`baton-file` is a [Baton](https://baton.conductorone.com) connector designed to ingest identity and access data directly from structured data files like Microsoft Excel (`.xlsx`), YAML (`.yaml`/`.yml`), and JSON (`.json`). It translates the data from specific structures or tabs within these files into Baton resources, entitlements, and grants.

This connector allows you to model users, groups, roles, applications, and their relationships defined in common file formats, making it suitable for scenarios where the managed application does not have a method to currently integrate directly with ConductorOne either through cloud or on-premises connectors. The `baton-file` connector allows for the modeling of even the most complex application resource relationship data models and provides visibility into its full permission/access inheritance model.

## Key Features

*   **Multiple File Formats:** Reads data directly from `.xlsx`, `.yaml`/`.yml`, and `.json` files.
*   **Structured Input:** Expects data organized into specific tabs (Excel) or top-level keys (YAML/JSON) (`users`, `resources`, `entitlements`, `grants`) with defined fields/columns.
*   **Explicit Trait Definition:** Uses the `Resource Function` field in the `resources` data to assign Baton traits (user, group, role, app, secret) to discovered resource types.
*   **Per-Sync Reloading:** Re-reads the input file data during every sync cycle.
*   **Standard Baton Functionality:** Supports both C1Z file generation and direct connector mode.
*   **Custom User Attribute Support:** Ingests user profile attributes via dedicated `Profile: *` columns (Excel) or nested `profile` objects (YAML/JSON).

## Getting Started

### Prerequisites

*   Familiarity with your application's identity and access data model.
*   An input file (`.xlsx`, `.yaml`, or `.json`) structured according to the expected format (see [File Formats](#file-formats) below). Example templates illustrating the structure are provided in the `templates/` directory.
*   Access to a ConductorOne instance (required for direct connector mode).

### Installation

#### Brew

```bash
# Add the ConductorOne tap (if you haven't already)
brew tap conductorone/baton

# Install Baton and the file connector
brew install baton conductorone/baton/baton-file

# Verify installation
baton-file --help
baton --help
```

#### Docker

```bash
# Pull the latest images
docker pull ghcr.io/conductorone/baton:latest
docker pull ghcr.io/conductorone/baton-file:latest

# Example: Run connector to generate C1Z file (replace path/to/your/data.file)
docker run --rm -v $(pwd):/out -e BATON_INPUT=path/to/your/data.file ghcr.io/conductorone/baton-file:latest

# Example: Run Baton against the generated C1Z
docker run --rm -v $(pwd):/out ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources
```

#### Source

```bash
# Install Baton CLI
go install github.com/conductorone/baton/cmd/baton@main

# Install baton-file connector
go install github.com/conductorone/baton-file/cmd/baton-file@main

# Verify installation
baton-file --help
baton --help
```

## Usage

The `baton-file` connector requires the path to your input data file specified via the `--input` (or `-i`) flag or the `BATON_INPUT` environment variable.

### One-shot Mode (C1Z Generation)

This is the default behavior when tenant client credentials are not provided.

```bash
# Example using Excel
baton-file -i templates/template.xlsx

# Example using YAML
baton-file -i templates/template.yaml

# Example using JSON
baton-file -i templates/template.json

# Specify output file name
baton-file -i path/to/your/data.file --file my-sync.c1z

```

This command will:
1.  Read and parse the data from the specified file.
2.  Process the resources, entitlements, and grants.
3.  Create a C1Z file (`sync.c1z` by default).

### Continuous Service Mode

Providing your ConductorOne tenant client ID and client secret via flags automatically triggers Continuous Service Mode. This mode is recommended for production deployments.

```bash
# Example using flags (replace path and credentials)
./bin/baton-file -i path/to/your/data.file \
  --client-id $BATON_CLIENT_ID \
  --client-secret $BATON_CLIENT_SECRET
```

In this mode, the connector starts, authenticates with ConductorOne, and waits for sync tasks. When a sync is triggered, it re-reads the input file data as needed for each phase of the sync.

### Standard Flags

`baton-file` supports standard Baton SDK flags:

*   `-i`, `--input`: **(Required)** Path to the input data file (`.xlsx`, `.yaml`, `.yml`, `.json`).
*   `-c`, `--client-id`: ConductorOne Client ID (for direct mode).
*   `-s`, `--client-secret`: ConductorOne Client Secret (for direct mode).
*   `--file`: Path to output C1Z file (default: `sync.c1z`).
*   `--log-level`: Set logging level (`debug`, `info`, `warn`, `error`).
*   `--log-format`: Set log format (`console` or `json`).
*   `-h`, `--help`: Display help information.

## File Formats & Data Structure

The connector expects an input file (`.xlsx`, `.yaml`, `.yml`, or `.json`) containing specific data structures. Templates are provided in the `templates/` directory:

*   [`./templates/template.xlsx`](./templates/template.xlsx)
*   [`./templates/template.yaml`](./templates/template.yaml)
*   [`./templates/template.json`](./templates/template.json)

Detailed instructions and explanations for each file format are available:

*   [Excel (`.xlsx`) Instructions](./docs/excel_instructions.md)
*   [YAML (`.yaml`/`.yml`) Instructions](./docs/yaml_instructions.md)
*   [JSON (`.json`) Instructions](./docs/json_instructions.md)

**Core Structure Summary:**

*   **Excel:** Data organized into specific tabs (`users`, `resources`, `entitlements`, `grants`) with defined columns. Column order does not matter, but header names must match required fields (case-insensitive for standard headers, case-sensitive for `Profile: *` keys after the prefix).
*   **YAML/JSON:** Data organized under top-level keys (`users`, `resources`, `entitlements`, `grants`), where each key holds a list of objects. Object keys must match expected field names (lowercase snake_case, e.g., `display_name`, `resource_type`).

### Data Sections

Refer to the detailed instruction files linked above for specific field names and requirements for each format.

1.  **`users`:** Defines all user resources (including service accounts).
2.  **`resources`:** Defines all non-user resources (groups, roles, apps, etc.) and their Baton trait (`Resource Function`).
3.  **`entitlements`:** Defines specific permissions, membership types, or role assignments on resources.
4.  **`grants`:** Defines which principals (users or group/role entitlements) are granted which entitlements.

## Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually building spreadsheets. We welcome contributions and ideas, no matter how small—our goal is to make identity and permissions sprawl less painful for everyone. If you have questions, problems, or ideas: Please open a GitHub Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.
