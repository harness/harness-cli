# Harness CLI Tool (hc)

A powerful command-line interface tool for interacting with Harness services

## Overview

The Harness CLI (hc) provides a unified command-line interface for interacting with various Harness services. It follows a consistent, resource-based command structure:

```
hc <resource> <action> <args> <flags>
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

### Binary Installation

Download the latest binary from the releases page:

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

# Install go tools:
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && go install golang.org/x/tools/cmd/goimports@latest && go install golang.org/x/vuln/cmd/govulncheck@latest && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && go install github.com/daixiang0/gci@latest

# Clone the repository
git clone https://github.com/harness/harness-cli.git
cd harness-cli

# Build the binary
make build
```

## Configuration

The CLI can be configured using:

1. Configuration file at `$HOME/.harness/auth.json`
2. Environment variables
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

# Get artifact details
hc artifact get <artifact-name> --registry <registry-name>

# Delete an artifact
hc artifact delete <artifact-name> --registry <registry-name>

# Push artifacts
hc artifact push generic <registry-name> <file-path> --name <artifact-name> --version <version>
hc artifact push maven <registry-name> <file-path>
hc artifact push go <registry-name> <module-path>

# Pull artifacts
hc artifact pull generic <registry-name> <package-path> <destination>
```

### Project Management (`hc project` or `hc proj`)

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

### Organisation Management (`hc organisation` or `hc org`)

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

### API Passthrough (`hc api`)

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

## Global Flags

The following flags are available for all commands:

```bash
--account string          Account ID (overrides saved config)
--api-url string          Base URL for the API (overrides saved config)
--token string            Authentication token (overrides saved config)
--org string              Organisation ID (overrides saved config)
--project string          Project ID (overrides saved config)
--format string           Output format: table (default) or json
--log-file string         Path to store logs
```

## Output Formatting

The CLI supports different output formats using the `--format` flag:

```bash
# Output in JSON format
hc registry list --format=json

# Output in table format (default)
hc registry list --format=table

# Works with all list/get commands
hc artifact list --registry my-reg --format=json
```

JSON output supports:
- Pretty printing with configurable indentation
- Smart pagination information
- Custom output formatting

## Development

### Project Structure

```
harness-cli/
├── api/              # OpenAPI specs for each service
├── cmd/              # CLI commands implementation
│   ├── hc/           # Main CLI entry point
│   ├── auth/         # Authentication commands
│   ├── registry/     # Registry management commands
│   ├── artifact/     # Artifact management commands
│   ├── project/      # Project management commands
│   ├── organisation/ # Organisation management commands
│   └── api/          # API passthrough command
├── config/           # Configuration handling
├── internal/         # Internal packages and generated API clients
├── module/           # Service-specific modules
├── tools/            # Development tools
└── util/             # Utility functions
```

### Adding New Commands

1. Add the OpenAPI spec in the `api/<service>` directory
2. Run the code generator to create service client:
   ```bash
   make generate
   ```
3. Create a new command package under `cmd/<resource-name>/`
4. Implement commands in `cmd/<resource-name>/command/`
5. Register the command in `cmd/hc/main.go`

### Building

```bash
# Build the binary
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run linter
make lint
```

## Examples

### Complete Workflow Example

```bash
# 1. Authenticate
hc auth login

# 2. List registries
hc registry list

# 3. Push an artifact
hc artifact push generic my-registry ./my-file.tar.gz \
  --name my-app \
  --version 1.0.0

# 4. List artifacts in the registry
hc artifact list --registry my-registry

# 5. Pull the artifact
hc artifact pull generic my-registry my-app/1.0.0/my-file.tar.gz ./downloads/

# 6. Check authentication status
hc auth status
```

### Using Aliases

```bash
# Registry commands
hc reg list
hc reg get my-registry

# Artifact commands
hc art list --registry my-registry
hc art push generic my-registry ./file.tar.gz --name app --version 1.0.0

# Project/Organisation commands
hc proj list
hc org list
```

## Migration from v1

If you're upgrading from v1 of the CLI, note the following command changes:

| Old Command (v1) | New Command (v2) |
|-----------------|------------------|
| `hc ar get registry` | `hc registry list` or `hc registry get <name>` |
| `hc ar get artifact` | `hc artifact list` or `hc artifact get <name>` |
| `hc ar push generic` | `hc artifact push generic` |
| `hc ar pull generic` | `hc artifact pull generic` |
| `hc ar delete registry` | `hc registry delete` |
| `hc ar delete artifact` | `hc artifact delete` |
| `hc ar migrate` | `hc registry migrate` |

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Apache 2.0
