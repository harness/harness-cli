# Registry CLI

Registry CLI is a command-line tool for migrating artifacts between different artifact registries. It currently supports migration from JFrog Artifactory to Harness Artifact Registry (HAR).

## Features

- Migrate artifacts between different registry types
- Configurable concurrent operations for efficient migration
- Filtering support for selective migration of artifacts
- Dry run mode to validate before actual migration
- Status tracking and monitoring of ongoing migrations
- Docker container support for easy deployment

## Installation

### Building from Source

```bash
git clone <repository-url>
cd registry
go build -o registry ./cmd/registry
```

### Using Docker

```bash
docker build -t registry .
```

## Configuration

Registry CLI uses a YAML configuration file to define the migration settings. An example configuration file is provided at `config.yaml.example`.

### Configuration Options

- `migration`: General migration settings
  - `dryRun`: Whether to perform a dry run without actual uploads
  - `concurrency`: Number of concurrent operations
  - `failureMode`: How to handle failures (`continue` or `stop`)
  
- `source`: Source registry configuration
  - `type`: Registry type (currently supports `JFROG`)
  - `endpoint`: API endpoint URL
  - `credentials`: Authentication details
  - `filters`: Artifact selection filters
  
- `destination`: Destination registry configuration
  - `type`: Registry type (currently supports `HAR`)
  - `accountIdentifier`: Account identifier for the destination
  - `registryEndpoint`: API endpoint URL
  - `credentials`: Authentication details
  - `mappings`: Registry mappings from source to destination

### Environment Variables

The configuration file supports environment variable substitution. Use `${VARIABLE_NAME}` syntax in the configuration file to reference environment variables.

## Usage

### Starting a Migration

```bash
./registry migrate --config config.yaml
```

Options:
- `--config, -c`: Path to the configuration file (default: `config.yaml`)
- `--api-url`: Override the API base URL from the config
- `--token`: Override the authentication token from the config
- `--dry-run`: Perform a dry run (overrides config setting)
- `--concurrency`: Set the number of concurrent operations (overrides config setting)

### Checking Migration Status

```bash
./registry status --id <migration-id>
```

Options:
- `--id, -i`: Migration ID (required)
- `--config, -c`: Path to the configuration file (default: `config.yaml`)
- `--api-url`: Override the API base URL from the config
- `--token`: Override the authentication token from the config
- `--poll`: Poll interval in seconds (0 for a single query)

### Using Docker

```bash
docker run -v $(pwd)/config.yaml:/root/config/config.yaml registry migrate --config /root/config/config.yaml
```

## API Integration

The Registry CLI integrates with the Artifact Registry Replication API:

- `POST /api/v1/migration/start`: Start a migration
- `GET /api/v1/migration/{id}/status`: Get migration status
- `PUT /api/v1/migration/{id}/update`: Update artifact status
- `POST /api/v1/migration/create-registry`: Create a registry
- `GET /api/v1/migration/create-registry/{id}`: Get registry information
- `GET /api/v1/migration/artifacts/{id}`: Get all artifacts in a registry

## License

[License information here]
