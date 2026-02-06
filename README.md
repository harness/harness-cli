# Harness CLI Tool (hc)

A powerful command-line interface tool for interacting with Harness services

## Overview

The Harness CLI (hc) provides a unified command-line interface for interacting with various Harness services. It follows a consistent, resource-based command structure:

```
hc [<global-flags>] <command> <subcommand> [<positional-args>…] [<flags>]
```

### Available Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `auth` | - | Authentication commands (login, logout, status) |
| `registry` | `reg` | Manage Harness Artifact Registries |
| `artifact` | `art` | Manage artifacts in registries |
| `project` | `proj` | Manage Harness Projects |
| `organisation` | `org` | Manage Harness Organisations |
| `api` | - | Raw REST API passthrough for power users |

## Installation

### Quick Install (Recommended)

Install the latest version with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/harness/harness-cli/v2/install | sh
```

Or with sudo if you need elevated privileges:

```bash
curl -fsSL https://raw.githubusercontent.com/harness/harness-cli/v2/install | sudo sh
```

This script automatically detects your OS and architecture, downloads the appropriate binary, verifies its checksum for security, and installs it to `/usr/local/bin`.

### Custom Installation Directory

You can install to a custom directory by setting the `INSTALL_DIR` environment variable:

```bash
curl -fsSL https://raw.githubusercontent.com/harness/harness-cli/v2/install | INSTALL_DIR=$HOME/.local/bin sh
```

### Manual Binary Installation

Download the latest binary from the [releases page](https://github.com/harness/harness-cli/releases):

```bash
# Download the latest release for your platform
# Make it executable
chmod +x hc
# Move it to a directory in your PATH
mv hc /usr/local/bin/
```

### Building from Source

```bash
# Install go if you haven't

# Clone the repository
git clone https://github.com/harness/harness-cli.git
cd harness-cli

# Build the binary
make build
```

## Configuration

The CLI can be configured using:

1. Configuration file at `$HOME/.harness/auth.json`
2. Environment variables (coming soon)
3. Command-line flags

### Authentication

Before using most commands, you need to authenticate:

```bash
# Login with API key
hc auth login

# Check authentication status
hc auth status

# Logout
hc auth logout
```

You can also provide credentials via:
- Configuration file at `$HOME/.harness/auth.json`
- Command-line flags: `--token`, `--account`, `--org`, `--project`
- Environment variables

## Command Reference

### Authentication (`hc auth`)

Manage authentication with Harness services.

```bash
# Login interactively
hc auth login

# Login with API key
hc auth login --api-key <your-api-key>

# Check authentication status
hc auth status

# Logout
hc auth logout
```

### Registry Management (`hc registry` or `hc reg`)

Manage Harness Artifact Registries.

```bash
# List all registries
hc registry list
hc reg list  # Using alias

# Get registry details
hc registry get <registry-name>

# Create a registry (coming soon)
hc registry create <registry-name> --package-type DOCKER

# Delete a registry
hc registry delete <registry-name>

# Migrate artifacts from external registries
hc registry migrate --config migrate-config.yaml
```

### Artifact Management (`hc artifact` or `hc art`)

Manage artifacts within registries.

```bash
# List all artifacts
hc artifact list
hc art list  # Using alias

# List artifacts in a specific registry
hc artifact list --registry <registry-name>

# Delete an artifact (deletes all versions)
hc artifact delete <artifact-name> --registry <registry-name>

# Delete a specific version of an artifact
hc artifact delete <artifact-name> --registry <registry-name> --version <version>

# Push artifacts
hc artifact push generic <registry-name> <file-path> --name <artifact-name> --version <version>
hc artifact push go <registry-name> <module-path>

# Pull artifacts
hc artifact pull generic <registry-name> <package-path> <destination>
```

### Project Management (`hc project` or `hc proj`) (coming soon)

Manage Harness Projects.

```bash
# List all projects
hc project list

# Get project details
hc project get <project-id>

# Create a project (coming soon)
hc project create <project-id>

# Delete a project (coming soon)
hc project delete <project-id>
```

### Organisation Management (`hc organisation` or `hc org`) (coming soon)

Manage Harness Organisations.

```bash
# List all organisations
hc organisation list
hc org list  # Using alias

# Get organisation details
hc org get <org-id>

# Create an organisation (coming soon)
hc org create <org-id>

# Delete an organisation (coming soon)
hc org delete <org-id>
```

### API Passthrough (`hc api`) (coming soon)

Make raw REST API calls to Harness (for power users).

```bash
# GET request
hc api /har/api/v1/registries

# POST request with data
hc api /har/api/v1/registries --method POST --data '{"identifier":"my-registry"}'

# Custom headers
hc api /har/api/v1/registries --header "Content-Type: application/json"

# PUT/DELETE requests
hc api /har/api/v1/registries/my-reg --method DELETE
```

## Interactive Mode

When you run `hc` with no arguments in a terminal, the CLI launches an interactive menu powered by [Charm](https://charm.land) libraries. The menu lets you browse available commands, fill in parameters with guided forms, and see styled output with colors, spinners, and tables.

### Launching Interactive Mode

```bash
# Auto-launches when no subcommand is given in a terminal
hc

# Explicitly request interactive mode
hc --interactive
hc -i
```

### Features

- **Main menu**: Navigate all commands with arrow keys, filter by typing
- **Interactive tables**: Registry and artifact lists with keyboard navigation and pagination
- **Confirmation prompts**: Destructive operations (e.g. `delete`) ask for confirmation before proceeding
- **Spinners & progress**: Visual feedback during API calls and long operations
- **Themed output**: Consistent color system with success/error/warning indicators

### Scriptability Guarantees

Interactive mode is **never** enabled when stdout is not a TTY (e.g. piped or redirected). This ensures scripts and CI pipelines always get plain, parseable output:

```bash
# These all produce plain output, never interactive prompts:
hc registry list | jq .
hc registry list > output.txt
hc registry list --json
CI=true hc registry list
```

## Global Flags

The following flags are available for all commands:

```bash
--account string          Account ID (overrides saved config)
--api-url string          Base URL for the API (overrides saved config)
--token string            Authentication token (overrides saved config)
--org string              Organisation ID (overrides saved config)
--project string          Project ID (overrides saved config)
--format string           Output format: table (default) or json
--json                    Output results as JSON (shorthand for --format=json)
--interactive, -i         Force interactive TUI mode (requires a terminal)
--no-color                Disable colour output (also respects NO_COLOR env)
--verbose, -v             Enable verbose logging to stderr
```

## Output Formatting 

The CLI supports different output formats using the `--format` flag:

```bash
# Output in JSON format
hc registry list --format=json
hc registry list --json    # shorthand

# Output in table format (default)
hc registry list --format=table

# Works with all list/get commands
hc artifact list --registry my-reg --format=json
```

JSON output supports:
- Pretty printing with configurable indentation
- Smart pagination information
- Custom output formatting

### Disabling Colours

Colours are automatically disabled when stdout is not a terminal. You can also
force plain output:

```bash
# Via flag
hc registry list --no-color

# Via environment variable (https://no-color.org)
NO_COLOR=1 hc registry list
```

## Development

### Project Structure

```
harness-cli/
├── api/                  # OpenAPI specs for each service
├── cmd/                  # CLI commands implementation
│   ├── hc/               # Main CLI entry point
│   ├── auth/             # Authentication commands
│   ├── registry/         # Registry management commands
│   ├── artifact/         # Artifact management commands
│   ├── project/          # Project management commands
│   ├── organisation/     # Organisation management commands
│   └── api/              # API passthrough command
├── config/               # Configuration handling
├── internal/
│   ├── api/              # Generated API clients (ar, ar_v2, ar_v3, ar_pkg)
│   ├── style/            # Lipgloss theme tokens (colours, borders, typography)
│   ├── terminal/         # TTY detection, colour capability helpers
│   └── tui/              # Bubble Tea models (menu, tables, spinners, forms)
├── module/               # Service-specific modules
├── tools/                # Development tools
└── util/                 # Utility functions (printer, progress, auth, etc.)
```

### Adding New Interactive Flows

To add an interactive TUI flow for a new command:

1. **Create a new model** in `internal/tui/` (e.g. `artifact_list.go`).
   Follow the pattern in `registry_list.go` — define a Bubble Tea `Model` with
   `Init`, `Update`, `View`, and a `Run*` entry-point function.

2. **Wire it into the menu** by adding a `menuItem` in `internal/tui/menu.go`
   and a case in `runInteractiveMode()` in `cmd/hc/main.go`.

3. **Use shared components**: spinners (`tui.RunWithSpinner`), confirmations
   (`tui.ConfirmDeletion`), text inputs (`tui.PromptInput`), and select
   prompts (`tui.PromptSelect`) are already available.

4. **Keep core logic separate**: the TUI model should call the same API
   functions used by the non-interactive Cobra command. Never duplicate
   business logic.

5. **Use the theme**: import `internal/style` for all colours and styles so
   the UI stays consistent.

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Run linter
make lint
```

## License

MIT License
