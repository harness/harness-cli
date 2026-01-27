# Feature: Artifact Version Delete Support

## Summary
Added support for deleting specific artifact versions (prune functionality) in the Harness CLI using an optional `--version` flag.

## Changes Made

### 1. Enhanced Delete Command
- **File**: `cmd/artifact/command/delete.go`
- Enhanced `hc artifact delete` command to accept optional `--version` flag
- **Without `--version`**: Deletes entire artifact and all its versions (existing behavior)
- **With `--version`**: Deletes only the specified version (new functionality)
- Uses the existing `DeleteArtifactVersion` API endpoint for version-specific deletion

### 2. Documentation
- **File**: `README.md`
- Updated artifact management section with the new `--version` flag
- Clarified behavior with and without the version flag
- Added examples for both use cases

## Usage

### Delete entire artifact (all versions):
```bash
hc artifact delete <artifact-name> --registry <registry-name>
```

### Delete a specific version:
```bash
hc artifact delete <artifact-name> --registry <registry-name> --version <version>
```

### Examples:
```bash
# Delete all versions of my-app artifact from my-registry
hc artifact delete my-app --registry my-registry

# Delete only version 1.0.0 of my-app
hc artifact delete my-app --registry my-registry --version 1.0.0

# Using alias
hc art delete my-app --registry my-registry --version 2.1.3
```

## Comparison

### Delete entire artifact (all versions):
```bash
hc artifact delete my-app --registry my-registry
# Deletes my-app and ALL its versions
```

### Delete specific version only:
```bash
hc artifact delete my-app --registry my-registry --version 1.0.0
# Deletes ONLY version 1.0.0 of my-app
```

## API Endpoint Used
- **Endpoint**: `/registry/{registry_ref}/+/artifact/{artifact}/+/version/{version}`
- **Method**: DELETE
- **Operation ID**: `DeleteArtifactVersion`

## Testing
Build and test the enhanced command:
```bash
make build
./hc artifact delete --help

# Test deleting all versions
./hc artifact delete my-app --registry my-registry

# Test deleting specific version
./hc artifact delete my-app --registry my-registry --version 1.0.0
```

## Design Benefits
- **Simpler UX**: Single command with optional flag instead of nested subcommands
- **Consistent with CLI patterns**: Similar to how other tools handle optional parameters
- **Backward compatible**: Existing `delete` behavior unchanged when `--version` not provided
- **Intuitive**: Natural progression from "delete artifact" to "delete artifact version"

## Notes
- The API endpoint already existed in the OpenAPI spec
- This feature was planned in `api/ar/command.yaml` but not implemented
- The implementation follows the same patterns as other artifact commands
- Proper error handling and logging included
- No breaking changes to existing functionality
