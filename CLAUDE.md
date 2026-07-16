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
  (`version.go:109`) build a lowercased `existingFileMap`, consumed in
  `Version.Migrate` (`version.go:180`). Source of that map:
  - If a prebuilt `types.ExistingIndex` was threaded in (overwrite=false, non-dry-run,
    `indexApplicable` type — see below), populate it from the index with **zero** API
    calls.
  - Otherwise fall back to `GetAllFilesForVersion` **once per version** — this
    per-version destination call *was* the dominant network cost, which the index
    eliminates.
- `File.Pre` (`file.go:111`): RAW does a HEAD via `FileExists`.

### Destination "existing artifacts" index (overwrite=false optimization)

`adapter.BuildExistingIndex` builds a read-only `types.ExistingIndex`
(`types/existing_index.go`) — `pkg → version → set<lowercased file>` plus a
`pkg → version` set — **once per registry** in `Registry.Migrate`, gated by
`!Overwrite && !DryRun && indexApplicable(type)` (`registry.go`). `indexApplicable` =
the exact set `Version.Pre` checks: GENERIC/RAW/PYTHON/NUGET/DART/PUPPET (MAVEN/NPM
excluded, mirroring `version.go:109`). The HAR impl (`har/client.go buildExistingIndex`)
composes the **`ar_v3`** listing endpoints (`internal/api/ar_v3`), not v1:
- Resolve registry **name → UUID** via `ListRegistriesV3` first. Neither
  `types.RegistryInfo` nor the v1 registry list carries a UUID, and the v3 batch
  params require one.
- `ListVersionsV3(registry_ids=[uuid])` returns **every version across all packages**
  in one paginated stream (batch), not one call per package like v1's
  `GetAllArtifactVersions`.
- Each `Version` row carries `FileCount`; call `ListFilesV3(version_id=…)` **only when
  `FileCount > 0`** (nil `FileCount` → treat as unknown, fetch anyway). Fresh migration
  ≈ zero per-version file calls.
- **v3 pagination differs from v1:** loop on `HasMore bool` (+ `Page`/`Size`), NOT
  v1's `PageCount`/`PageIndex`.
- Per-version file errors are **best-effort (log & continue)**; any top-level build
  error → `(nil, err)` → caller uses the per-version live fallback (today's behavior).
  A cache miss only ever triggers an idempotent re-upload, never data loss.
- Non-HAR adapters stub `BuildExistingIndex` as `(nil, nil)` (they are never the
  destination). The engine builds its own clients in `har/client.go newClient`
  (separate from the cmd-layer `cmdutils.Factory`); auth via `auth.GetXApiKeyOptionARV3`.

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

- `docs/AH-destination-index-v3-plan.md` — **implemented**: the destination
  `ExistingIndex` cache built on the *current* job tree via `ar_v3` batch endpoints
  (see "Destination index" section above). Independent of the `handler/` refactor.
- `docs/migration-type-handler-factory-plan.md` — **master plan (not implemented)**:
  refactor per-type logic into a `handler/` factory (Arc A, behavior-preserving), then
  layer the cache on top (Arc B). Note: the cache actually shipped on the current job
  tree per the v3 plan above, *not* via Arc B. The `handler/` package
  (`migratable/../handler/`) is an A0 skeleton only — registered nowhere, unwired.
- `docs/AH-4458-destination-index-plan.md` — earliest narrower cache plan,
  **superseded** (assumed v1-client composition; the v3 plan replaced it).
- `docs/nuget-migration-optimization.md`, `docs/tag-move-digest-sync.md`,
  `docs/migration-crash-analysis.md` — targeted fixes/analysis.

## Build / test

- `go build ./...` — build everything.
- `go test ./module/ar/migrate/...` — migration engine tests.
- `mock_jfrog` is the integration harness for end-to-end migration behavior.
- Commits reference JIRA (e.g. `[AH-4458]`); PRs target the `develop` branch.
</content>
