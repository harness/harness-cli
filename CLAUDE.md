# CLAUDE.md

Guidance for working in this repo (Harness CLI). Focused on the artifact-registry
**migration engine**, which is the most intricate subsystem.

## Repo layout (top level)

- `cmd/` — Cobra command trees (auth, registry, artifact, project, org, pkgmgr, …).
  `cmd/artifact/command/` holds per-ecosystem push commands (`push_maven.go`,
  `push_npm.go`, `push_nuget.go`, …) and `utils/`.
- `internal/api/ar/client_gen.go` + `internal/api/ar_pkg/client_gen.go` —
  **generated** OpenAPI clients (do not hand-edit). `ar` = HAR control-plane API
  (registries/artifacts/versions/files); `ar_pkg` = package upload/content API.
- `module/ar/migrate/` — the migration engine (see below).
- `util/`, `config/`, `api/` — shared helpers, global config, API glue.
- `docs/` — design/plan docs (see key ones below).

## Migration engine — `module/ar/migrate/`

Moves artifacts from a **source** registry (JFrog/Nexus) to a **destination**
(Harness HAR). Entry point: `migration.go` → `MigrationService.Run`.

### Job tree & engine

`engine/Engine.go` runs a bounded-concurrency worker pool over `engine.Job`s.
`Job` interface (`engine/job.go`): `Info() / Pre() / Migrate() / Post()`. Each
step is recover-guarded; errors are joined. Jobs **recursively spawn child jobs**
on their own nested `Engine` (concurrency from `config.Concurrency`).

The hierarchy is **Registry → Package → Version → File**, in
`migratable/{registry,package,version,file}.go`:

- `Registry.Migrate` (`registry.go:117`): source `GetFiles` → filters
  (date/include/exclude) → `tree.TransformToTree` → source `GetPackages` → spawn
  `Package` jobs.
- `Package.Migrate` (`package.go:218`): a **13-branch `if/else` on artifactType**
  (`package.go:240-291`) — either migrates directly (OCI, Helm variants, RPM,
  Debian, Conda, Composer, Swift, Conan) or `GetVersions` → spawn `Version` jobs.
- `Version.Migrate` (`version.go:129`): file-based types iterate `tree.GetAllFiles`
  → spawn `File` jobs, with **per-type filters** (NPM `.tgz`-only `version.go:155`,
  NUGET `version.go:160`, PUPPET `version.go:170`); GO downloads `.mod/.zip/.info`
  → `CreateVersion`; OCI unsupported at this layer.
- `File.Migrate` (`file.go:147`): per-type upload bodies (GENERIC/RAW/MAVEN/NUGET/
  PUPPET, PYTHON, NPM, DART).

### Skip / existence checks live in the `Pre` steps (overwrite=false)

- `Package.Pre` (`package.go:111`): OCI per-tag `remote.Head`/digest loop;
  HELM_LEGACY/HELM_HTTP `VersionExists`.
- `Version.Pre` (`version.go:88`): file-based types **except MAVEN/NPM**
  (`version.go:109`) call `GetAllFilesForVersion` **once per version** and build a
  lowercased `existingFileMap`, consumed in `Version.Migrate` (`version.go:180`).
  **This per-version destination call is the dominant network cost.**
- `File.Pre` (`file.go:111`): RAW does a HEAD via `FileExists`.

### Adapters — `adapter/`

`Adapter` interface: `adapter/adapter.go:15`. Self-register via `init()` into a
factory map (`adapter/adapter.go:72-101`); blank-imported in `migration.go:21-24`.
- `adapter/jfrog/` — source enumeration. `GetPackages` (`jfrog/adapter.go:105`,
  large type switch: NuGet name parsing, helm-index, OCI catalog) and `GetVersions`
  (`jfrog/adapter.go:1001`).
- `adapter/har/` — destination. `adapter.go` delegates to `client.go`, which holds
  all upload bodies + existence calls: `artifactVersionExists` (`client.go:1095`),
  `artifactFileExists` (`client.go:971`), `artifactGetFilesForVersion`
  (`client.go:1014`), `headRawFile` (`client.go:146`).
- `adapter/nexus/` — source. `adapter/mock_jfrog/` — **integration test harness**
  with canned testdata; delegates to `jfrog.NewAdapterWithClient`.

### Key conventions & gotchas

- **Case handling:** only **file names** are lowercased for existence matching
  (`version.go:118/180`); package/version names are matched **verbatim**.
- **Shared state is pointer + mutex:** `types.TransferStats` and `types.DryRunStats`
  are shared across concurrent jobs; always mutate via their methods
  (`types/types.go`). Reads after a build barrier (`g.Wait()`) are lock-free.
- **Idempotency:** duplicate uploads return `types.ErrArtifactAlreadyExists`
  (mapped from HTTP 409); callers treat it as a Skip, not a failure. A cache/lookup
  miss therefore only causes a safe re-upload attempt, never data loss.
- **Dry-run:** `--dry-run` skips all destination calls and emits
  `dry-run-output/{file_list,directory_structure}_*.json` (`migration.go:147`).
  Use a before/after diff of these as a **regression gate** for refactors.
- **Pagination idiom** (HAR client): `page` from 0, `size=100`; stop when
  `len(page) < size` OR `PageIndex+1 >= PageCount` (`client.go:1006/1041/1087/1136`).
- **API clients are generated** — extend behavior in the `har`/`jfrog` adapter
  wrappers, not in `*_gen.go`.

### Type → level matrix

Migration level (where upload happens) and skip level (existence granularity) vary
per type. See the full table in
`docs/migration-type-handler-factory-plan.md §2.4`. Summary:
- **Package-level migrate:** DOCKER/HELM (OCI, skip=Tag), HELM_LEGACY/HELM_HTTP
  (skip=Version), RPM/DEBIAN/CONDA/COMPOSER/SWIFT (skip=server-side), CONAN.
- **Version-level:** GO. **File-level:** GENERIC/RAW/MAVEN/PYTHON/NUGET/DART/PUPPET.
- **No client-side skip:** NPM, MAVEN (excluded at `version.go:109`).

## Active design docs

- `docs/migration-type-handler-factory-plan.md` — **master plan**: refactor
  per-type logic into a `handler/` factory (Arc A, behavior-preserving), then add a
  destination `ExistingIndex` cache to eliminate per-version lookups (Arc B).
- `docs/AH-4458-destination-index-plan.md` — earlier narrower cache plan,
  **superseded** by the master plan (its cache mechanics are reused in Arc B).
- `docs/nuget-migration-optimization.md`, `docs/tag-move-digest-sync.md`,
  `docs/migration-crash-analysis.md` — targeted fixes/analysis.

## Build / test

- `go build ./...` — build everything.
- `go test ./module/ar/migrate/...` — migration engine tests.
- `mock_jfrog` is the integration harness for end-to-end migration behavior.
- Commits reference JIRA (e.g. `[AH-4458]`); PRs target the `develop` branch.
</content>
