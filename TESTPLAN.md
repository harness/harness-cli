# E2E Migration Test Plan

Live integration tests for `hc registry migrate`: **MOCK_JFROG → HAR** (default) plus optional **JFROG OCI → HAR** for DOCKER/HELM.

## Running

```bash
# Type-check only (no credentials)
make e2e-vet

# Full suite (requires QA env + mock-init)
make e2e-migration

# Single package
make e2e-migration-pkg PKG=filters

# Offline OCI mechanics (no credentials, no e2e tag)
go test ./tests/ocismoke/...
```

### Required environment (live HAR)

| Variable | Purpose |
|----------|---------|
| `HARNESS_API_KEY` | QA PAT |
| `HARNESS_ACCOUNT_ID` | Account |
| `HARNESS_API_URL` | NG API base |
| `HARNESS_PKG_URL` | Package registry URL |
| `HARNESS_ORG_ID` | Optional; defaults to e2e org |
| `HARNESS_PROJECT_ID` | Optional; defaults to e2e project |
| `E2E_RUN_ID` | Optional; stable run suffix for CI |

### Optional environment (live OCI source)

| Variable | Purpose |
|----------|---------|
| `E2E_OCI_SOURCE_ENDPOINT` | JFrog Artifactory URL |
| `E2E_OCI_SOURCE_REGISTRY` | Source repo key |
| `E2E_OCI_SOURCE_USERNAME` | Source username |
| `E2E_OCI_SOURCE_PASSWORD` | Source token/password |
| `E2E_OCI_SOURCE_IMAGE` | Image/chart name |
| `E2E_OCI_SOURCE_TAG` | Tag to migrate |
| `E2E_OCI_SOURCE_PACKAGE_HOSTNAME` | Optional OCI hostname override |
| `E2E_OCI_SOURCE_INSECURE` | Set to `1` for HTTP sources |

## Phase status

| Phase | Scope | Package(s) | Status |
|-------|-------|------------|--------|
| **0** | Harness, Makefile, CI | `tests/harness/`, `.github/workflows/e2e-migration.yaml` | Done |
| **1** | Missing happy paths | `npm` (scoped), `raw` (shallow) | Done |
| **2** | Filters + path layout | `filters`, `pathlayout` | Done |
| **3** | Infra + reconciliation | `infra`, `reconciliation` | Done |
| **4** | OCI | `ocismoke` (offline), `docker`, `helm` (live, gated) | Done |
| **5** | Idempotency + failure | `idempotency`, `failure` | Done |
| **6** | Dest mismatch + enumeration | `destmismatch`, `enumeration` | Done |

## Test matrix (by package)

### Happy path (existing + Phase 1)

| Package | Source → Dest | Notes |
|---------|---------------|-------|
| `composer` | composer-local → COMPOSER | |
| `conda` | conda-local → CONDA | |
| `dart` | dart-local → DART | |
| `debian` | debian-local → DEBIAN | |
| `generic` | generic-local → GENERIC | |
| `gomod` | go-local → GO | |
| `helmhttp` | helm-http-local → GENERIC | nested chart |
| `helmlegacy` | helm-legacy-local → HELM | index.yaml |
| `maven` | maven-local → MAVEN | |
| `npm` | npm-local → NPM | lodash |
| `npm` (scoped) | npm-local → NPM | @har/sample-package |
| `nuget` | nuget-local → NUGET | |
| `puppet` | puppet-local → PUPPET | |
| `python` | python-local → PYTHON | |
| `raw` | raw-local → GENERIC | |
| `raw` (shallow) | raw-local → GENERIC | shallow rejected |
| `rpm` | rpm-local → RPM | |
| `swift` | swift-local → SWIFT | |

### Filters (`tests/filters`)

- Include / exclude (RAW, GENERIC, SWIFT)
- Deep `**` glob
- Zero-file filter
- Date filter ANY / ALL
- Date filter + include intersection

### Path layout (`tests/pathlayout`)

- GENERIC nested paths (no `%2F` encoding)
- RAW deep path
- Maven GAV + per-version file membership

### Infrastructure (`tests/infra`)

- Scope: account / org / project
- Multi-mapping config
- Concurrency > 1
- Dry-run output files, no upload
- Summary flag
- Token expansion (`${HAR_TOKEN}`)
- Invalid PAT on create
- CLI exit 0 on partial per-file failure

### Reconciliation (`tests/reconciliation`)

- RPM version format (not bare semver)
- Maven per-version file membership
- Negative reconcile on empty registry

### Idempotency (`tests/idempotency`)

- RAW skip on re-run (overwrite=false)
- Python version skip on re-run
- RAW overwrite=true re-processes

### Failure (`tests/failure`)

- RAW shallow path Failed + deep path Success (in-process stats)
- Source download 404 → Failed

### Dest mismatch (`tests/destmismatch`)

- Maven → GENERIC (version absent)
- Python → NUGET (version absent)
- Unsupported package type create fails

### Enumeration (`tests/enumeration`)

- HELM_HTTP tree-only chart (not in index)
- Empty registry → zero files

### OCI

| Package | Layer | Notes |
|---------|-------|-------|
| `ocismoke` | Offline | CopyRepository, ListTags, multi-arch, no-clobber |
| `docker` | Live e2e | Requires `E2E_OCI_SOURCE_*` |
| `helm` | Live e2e | Native HELM OCI; requires `E2E_OCI_SOURCE_*` |

## CI

`.github/workflows/e2e-migration.yaml`:

- **vet** job: `make e2e-vet` on every run
- **e2e** job: `make e2e-migration` (nightly + manual dispatch)
- Secrets: `E2E_HARNESS_*` (see workflow file)

## Known limitations

1. **HTTP mock OCI + HTTPS HAR**: `crane.Insecure` applies to both refs in the production copy path, so an in-memory HTTP mock cannot drive a live HAR migration end-to-end. Offline mechanics are covered by `tests/ocismoke`; live DOCKER/HELM use a real JFrog OCI source.
2. **Go missing .mod/.info**: Version enumeration keys off `.zip` presence; incomplete module triples are server-behavior-dependent — not asserted in e2e.
3. **DEBIAN type enablement**: Dest-mismatch for disabled package types depends on the QA environment.
