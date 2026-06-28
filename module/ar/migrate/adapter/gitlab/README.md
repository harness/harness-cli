# GitLab Package Registry Adapter

This adapter enables migration of artifacts from GitLab Package Registry to Harness Artifact Repository.

## Supported Package Types

- **Maven** - Java/JVM packages
- **NPM** - Node.js packages  
- **PyPI** - Python packages
- **NuGet** - .NET packages
- **Composer** - PHP packages
- **Conan** - C/C++ packages
- **Helm** - Kubernetes charts
- **Debian** - Debian packages
- **Generic** - Generic file packages
- **Go** - Go modules
- **RubyGems** - Ruby packages
- **Swift** - Swift packages

## Configuration

### Example Configuration File

```yaml
version: 1.0.0
concurrency: 5
overwrite: false

source:
  endpoint: https://gitlab.com  # or your self-hosted GitLab instance
  type: GITLAB
  credentials:
    username: gitlab-username  # Can be any username for token auth
    password: glpat-xxxxxxxxxxxx  # GitLab Personal Access Token
  insecure: false

destination:
  endpoint: https://pkg.harness.io
  type: HAR
  credentials:
    username: harness_user
    password: harness_api_key

mappings:
  - artifactType: MAVEN
    sourceRegistry: mygroup/myproject  # GitLab project path
    destinationRegistry: harness-maven

  - artifactType: NPM
    sourceRegistry: mygroup/myproject
    destinationRegistry: harness-npm

  - artifactType: PYTHON
    sourceRegistry: mygroup/myproject
    destinationRegistry: harness-pypi
```

### Authentication

GitLab authentication requires a **Personal Access Token** with appropriate scopes:

1. Go to GitLab → User Settings → Access Tokens
2. Create a new token with the following scopes:
   - `read_api` - Read API access
   - `read_registry` - Read package registry
   - `read_repository` - Read repository (for some package types)

3. Use the token as the `password` field in the configuration

### Source Registry Format

The `sourceRegistry` field should be the **project path** in GitLab:

- For project: `mygroup/myproject`
- For nested group: `mygroup/subgroup/myproject`
- For user project: `username/myproject`

## Usage

```bash
# Run migration
hc registry migrate -c gitlab-config.yaml

# Dry run (preview what would be migrated)
hc registry migrate -c gitlab-config.yaml --dry-run

# With custom concurrency
hc registry migrate -c gitlab-config.yaml --concurrency 10
```

## GitLab API Endpoints Used

The adapter uses the following GitLab API v4 endpoints:

- `GET /api/v4/projects/:id` - Get project information
- `GET /api/v4/projects/:id/packages` - List packages
- `GET /api/v4/projects/:id/packages/:package_id/package_files` - List package files
- `GET /api/v4/projects/:id/packages/:package_id/package_files/:file_id` - Download files

## Package Type Specifics

### Maven
- Packages use `groupId:artifactId` naming
- Supports maven-metadata.xml, pom files, and checksums
- Download path: `/api/v4/projects/:id/packages/maven/*path`

### NPM
- Supports scoped packages (@scope/package)
- Package names are normalized to lowercase
- Download path: `/api/v4/projects/:id/packages/npm/*path`

### PyPI
- Package names normalize underscores and dashes
- Supports wheel and source distributions
- Download path: `/api/v4/projects/:id/packages/pypi/files/*path`

### NuGet
- Package IDs are case-insensitive
- Supports .nupkg files
- Download path: `/api/v4/projects/:id/packages/nuget/download/*path`

### Generic
- General-purpose package storage
- Any file type supported
- Download path: `/api/v4/projects/:id/packages/generic/*path`

## Limitations

1. **Project-level only** - Currently supports project-level package registries. Group and instance-level registries are not yet supported.

2. **Pagination** - Large repositories with thousands of packages may take time to enumerate. The adapter handles pagination automatically.

3. **Rate Limiting** - GitLab.com has rate limits. For large migrations, consider:
   - Running during off-peak hours
   - Using a self-hosted GitLab instance
   - Adjusting concurrency settings

4. **Docker/Container Registry** - Container images in GitLab Container Registry use the OCI standard and should work, but may require additional testing.

## Troubleshooting

### Authentication Errors

```
GitLab API error (status 401): Unauthorized
```

**Solution:** Verify your Personal Access Token has the correct scopes (`read_api`, `read_registry`).

### Project Not Found

```
GitLab API error (status 404): Project not found
```

**Solution:** Verify the project path is correct. Use the full path including groups (e.g., `mygroup/myproject`).

### Rate Limiting

```
GitLab API error (status 429): Too Many Requests
```

**Solution:** Reduce concurrency or wait before retrying. Self-hosted GitLab instances have higher rate limits.

## Development

### Running Tests

```bash
cd module/ar/migrate/adapter/gitlab
go test -v
```

### Adding Support for New Package Types

1. Add the package type mapping in `mapArtifactTypeToGitLab()` and `mapGitLabPackageType()`
2. Create a new handler in `package_handlers.go`
3. Update the documentation

## References

- [GitLab Package Registry API Documentation](https://docs.gitlab.com/ee/api/packages.html)
- [GitLab Package Registry User Guide](https://docs.gitlab.com/ee/user/packages/package_registry/)
- [GitLab Personal Access Tokens](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html)
