# Harness CLI (hns)

A CLI tool for interacting with Harness services, following the design patterns similar to `gcloud` or `kubectl`.

## Command Structure

The CLI follows the command-tree grammar:

```
hns <service> <subcommand> <flags>
```

Currently implemented services:
- `ar`: AR service

## Installation

```bash
# Build the CLI
make build

# The binary will be created at bin/hns
```

## Usage Examples

### AR Service Commands

List all resources:
```bash
hns ar list
```

List all resources in JSON format:
```bash
hns ar list --format=json
```

Get a specific resource:
```bash
hns ar get res-001
```

Create a new resource:
```bash
hns ar create --name="New Resource" --description="A description of the new resource"
```

Delete a resource:
```bash
hns ar delete res-001
```

## Development

### Adding New Services

1. Create an OpenAPI spec for the service in the `api-specs` directory
2. Run `make generate` to generate code from the OpenAPI specs
3. Implement the command and operations for the service in the `pkg/services` directory
4. Add the service command to the root command in `cmd/hns/main.go`

### Updating Existing Services

1. Update the OpenAPI spec in the `api-specs` directory
2. Run `make generate` to regenerate code from the updated specs
3. Update the service implementation as needed

## Project Structure

```
harness-cli/
├── api-specs/           # OpenAPI specs for each service
│   └── ar.yaml
├── bin/                 # Compiled binaries
│   └── hns
├── cmd/                 # CLI command entry points
│   └── hns/
│       └── main.go
├── generated/           # Generated code from OpenAPI specs
├── pkg/                 # Package code
│   ├── services/        # Service implementations
│   │   └── ar/
│   │       ├── command.go
│   │       └── operations.go
│   └── util/            # Utility functions
│       └── format.go
├── tools/               # Development tools
│   └── generator/       # OpenAPI code generator
│       └── main.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```
