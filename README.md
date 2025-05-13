# Harness CLI Tool (hns)

A powerful command-line interface tool for interacting with Harness services
## Overview

The Harness CLI (hns) provides a unified command-line interface for interacting with various Harness services. It follows a consistent command structure:

```
hns <service> <subcommand> <flags>
```

## Installation

### Binary Installation

Download the latest binary from the releases page:

```bash
# Download the latest release for your platform
# Make it executable
chmod +x hns
# Move it to a directory in your PATH
mv hns /usr/local/bin/
```

### Building from Source

```bash
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
# `hns --api-token <command> 
```

## Available Services

### Artifact Registry (ar)

Commands for interacting with the Harness Artifact Registry.

```bash
# List all registries
hns ar get registries

# Get information about a specific artifact
hns ar get artifact <artifact-name> --registry=<registry-name>

# Get version information
hns ar get version <version> --registry=<registry-name> --artifact=<artifact-name>

# Push generic artifact
hns ar push generic --registry=<registry-name> --artifact=<artifact-name> --version=<version> --file=<file-path>

# Push maven artifact
hns ar push maven --registry=<registry-name> --artifact=<artifact-name> --version=<version> --file=<file-path>

# Delete a registry
hns ar delete registry <registry-name>

# Delete an artifact
hns ar delete artifact <artifact-name> --registry=<registry-name>

# Delete a version
hns ar delete version <version> --registry=<registry-name> --artifact=<artifact-name>
```

## Output Formatting

The CLI supports different output formats using the `--format` flag:

```bash
# Output in JSON format
hns ar get registries --format=json

# Output in table format (default)
hns ar get registries --format=table
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
