# Harness CLI Tool (hc)

A powerful command-line interface tool for interacting with Harness services
## Overview

The Harness CLI (hc) provides a unified command-line interface for interacting with various Harness services. It follows a consistent command structure:

```
hc <service> <subcommand> <flags>
```

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

The CLI supports authentication with Harness services using an API key:

```bash
# Set API key via environment variable
export HARNESS_API_TOKEN=your_api_token

# Or include it in your configuration file
# $HOME/.harness/auth.json

# Or provide it as a flag
# `hc --api-token <command> 
```

## Available Services

### Artifact Registry (ar)

Commands for interacting with the Harness Artifact Registry.

```bash
# List all registries
hc ar get registry

# Get information about a specific artifact
hc ar get artifact [?artifact-name] --registry=<registry-name>

# Get version information
hc ar get version [?version] --registry=<registry-name> --artifact=<artifact-name>

# Push generic artifact
hc ar push generic [registry-name] [file-path] --name <artifact-name> --version=<version>

# Pull generic artifact
hc ar pull generic [registry-name] [artifact-path] [destination]

# Delete a registry
hc ar delete registry [registry-name]

# Delete an artifact
hc ar delete artifact [artifact-name] --registry=<registry-name>

# Delete a version
hc ar delete version [version-name] --registry=<registry-name> --artifact=<artifact-name>
```

## Output Formatting

The CLI supports different output formats using the `--format` flag:

```bash
# Output in JSON format
hc ar get registries --format=json

# Output in table format (default)
hc ar get registries --format=table
```

JSON output supports:
- Pretty printing with configurable indentation
- Smart pagination information
- Custom output formatting

## Development

### Project Structure

```
harness-cli/
├── api-specs/        # OpenAPI specs for each service
├── cmd/              # CLI commands implementation
│   ├── ar/           # Artifact Registry commands
│   └── common/       # Common functionality across commands
├── config/           # Configuration handling
├── internal/         # Internal packages
├── module/           # Service-specific modules
├── tools/            # Development tools
│   └── generator/    # OpenAPI code generation
└── util/             # Utility functions

```

### Adding New Services

1. Add the OpenAPI spec in the `api/<service>` directory
2. Run the code generator to create service client:
   ```
   make generate
   ```
3. Implement the service commands under `cmd/<service-name>`

### Building

```bash
make build
```
