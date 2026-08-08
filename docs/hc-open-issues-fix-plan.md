# `hc` open issues — fix plan

**Companion to** `harness-hc-cli-open-issues_md.md` (the defect register). This document plans the
remediation work for each open issue. **No implementation has been done.** File:line references were
verified against the working tree on **2026-08-07**.

## 0. What the code review changed about the defect register

Two findings from mapping the current tree materially reshape the work:

1. **P1 is partially fixed in-tree.** `validateConfig` already rejects an unknown `artifactType`
   at config load with a non-zero exit and a valid-values list
   (`module/ar/migrate/types/config.go:188-189`), and TERRAFORM is already in
   `knownArtifactTypes` (`types/config.go:45,56`). The silent zero-work success from a typo'd type
   can no longer reach source enumeration. What remains of P1 is narrower: the `--help` text
   (`cmd/registry/migrate_cmd.go:88-89`) still omits TERRAFORM, and a **non-empty source mapping
   that resolves to zero packages** is still not a hard failure at the process level (see §1).
2. **P2d is mostly fixed in-tree.** Include/exclude patterns **are** applied
   (`migratable/registry.go:223-231` file-level via `FilterFilesByPatterns`,
   `registry.go:297-304` package-level via `FilterFilesByPatternsPackageName`; helpers in
   `util/patternUtil.go:91-162`). The `// NOT IMPLEMENTED YET` comment at `types/config.go:108-109`
   is stale. What remains: unsupported-type configs are **not** rejected (patterns on
   DEBIAN/TERRAFORM/PUPPET are silently ignored), and the stale comment/documentation must be fixed.

Everything else in the register matches the code: `Run` swallows engine errors and returns nil
(exit 0 after failures), the jfrog adapter dumps the full file listing at info level, explicit-flag
push never derives `PkgURL` from `--api-url`, dry-run delete prompts, `registry create` is a stub,
and `artifact list` ignores positional registry args.

## Priority order (recommended)

| Order | Issue | Why first |
|---|---|---|
| 1 | P2a truthfulness / exit codes (§2) | Bridge-completeness evidence depends on it; largest blast radius |
| 2 | P2b push URL construction (§3) | Only workaround (`--pkg-url`) is deprecated while the root cause is open |
| 3 | P2f dry-run delete trap (§4) | Operator-safety defect that already caused real damage |
| 4 | P1 residual gaps (§1) | Small; config-load validation already landed |
| 5 | P2c delete verification (§5) | Needs server-side cooperation; CLI can at least stop claiming false success |
| 6 | P2d residuals (§6) | Comment + reject-unsupported-filters |
| 7 | P2e date-filter documentation/UX (§7) | Operational rule, not a code fix |
| 8 | P3c bounded verbose output (§8) | Usability |
| 9 | P3b registry/list commands (§9) | Usability, workarounds exist |
| 10 | P3a TERRAFORM/RUBY gaps (§10) | Capability gap; only matters if the bridge must replicate them |

Each section below is scoped to be an independent PR (commit per JIRA convention, `[AH-XXXX]`).

---

## 1. P1 — artifactType validation (residual work)

**Already done (verify and close with tests):**
- `validateConfig` rejects unknown types pre-enumeration: `module/ar/migrate/types/config.go:188-189`
  (tests at `types/config_validation_test.go:61-76`).

**Remaining work:**

- [ ] **W1. Fix `--help` text** — add TERRAFORM to the supported-types list at
  `cmd/registry/migrate_cmd.go:88-89` (validation error string already includes it). Drive both
  from a single source of truth: export a `types.KnownArtifactTypes()` slice and render it in the
  help template instead of maintaining two literals.
- [ ] **W2. Zero-package hard failure** — strengthen the guard at
  `migratable/registry.go:262-279` (source has files but 0 packages and no filters → error) so it
  propagates to a **non-zero exit** (depends on §2 exit-code work — the engine collects the error
  today but `Run` still returns nil). Confirm the guard also fires when include/exclude patterns
  are set but the *source enumeration itself* is type-mismatched.
- [ ] **W3. Regression test** — config with `artifactType: NOTAREALTYPE` must exit non-zero before
  any source call (integration: `adapter/mock_jfrog`).

**Acceptance:** typo'd type → explicit error + non-zero exit + zero source API calls; `--help`
lists exactly the validated enum.

---

## 2. P2a — truthful totals, typed failures, non-zero exit

This is the keystone item: every other "silent success" defect funnels through here.

**Code map (verified):**
- `MigrationService.Run` logs engine errors and unconditionally `return nil` (non-dry-run):
  `module/ar/migrate/migration.go:97-131`; summary print at `migration.go:134-145`.
- Cmd layer prints "Migration completed successfully" whenever `Run` returns nil:
  `cmd/registry/migrate_cmd.go:182-186` (uses `Run` + `log.Fatalf`, not `RunE`).
- `TransferStats`/`DryRunStats` are already mutex-guarded (`types/types.go:84-112, 194-299`) — the
  old append race appears addressed; **verify under `-race`** rather than assume.
- Swallowed-error sites: nested package engine errors logged then `Registry.Migrate` returns nil
  (`registry.go:362-366`); `GetVersions` failures produce **no** `FileStat` row
  (`package.go:276-279`); nested version engine errors logged, `Package.Migrate` returns nil
  (`package.go:286-290`).

**Work items:**

- [ ] **W1. Typed failure for every enumeration path** — on `GetPackages`/`GetVersions` failure,
  record a `FileStat` (or new `PackageStat`) with `StatusFail` via
  `util.AddPackageErrorToStat` (`util/utils.go:200-209`) instead of log-and-continue at
  `registry.go:362-366` and `package.go:276-290`. A whole-package abort must appear in Failed.
- [ ] **W2. Exit-code contract** — `Run` returns a non-nil error (aggregate) when
  (a) the engine returned errors, **or** (b) any `StatusFail` stat exists. Keep dry-run behavior
  unchanged. `runMigration` then `log.Fatalf`s → exit 1. **Decided (2026-08-07): no opt-out flag —
  an unsuccessful migration fails the process, period.** The bridge must consume the per-coordinate
  result file (W4) / read-API reconciliation rather than relying on exit-0-with-failures.
- [ ] **W3. Distinguish skip classes** — the enum already has `Skipped` (`types/types.go:69-73`);
  ensure `AlreadyPresent`/index skips vs filter-excluded are distinguishable in the stat `Error`/
  reason field so reconciliation tooling can rely on it.
- [ ] **W4. Machine-readable result** — emit a per-coordinate JSON-lines result file (opt-in flag,
  e.g. `--result-file`), parallel to the existing dry-run output convention
  (`migration.go:147` area). Bridge completeness evidence should not parse human tables.
- [ ] **W5. Race audit** — run `go test -race ./module/ar/migrate/...` plus a high-concurrency
  mock_jfrog integration run; fix any remaining shared-counter issues in
  `types/types.go` / `engine`.
- [ ] **W6. Reconciliation mode (stretch)** — post-run source↔destination comparison command
  (read-only). Explicitly **not** a substitute for W1–W2.

**Acceptance:** any enumeration/download/upload failure → counted Failed + non-zero exit;
`Success/Skipped/Failed/Total` reconcile exactly against the JSON-lines result file; `-race` clean.

---

## 3. P2b — explicit-credential push builds wrong package root; `--pkg-url` deprecation

**Root cause (verified):** package clients are built **only** from
`config.Global.Registry.PkgURL` (`cmd/cmdutils/factory.go:32-34,71-73`). `loadAuthConfig` fills
`PkgURL` solely from `auth.json`'s `registry_url` (`cmd/hc/main.go:136-141`); `--api-url` never
derives it. With explicit `--account/--api-url/--token` and no `--pkg-url`, the request goes to a
stale/empty package host → `404 {"message":"Root not found: <account>"}` after the bytes stream.
Deprecation notice lives in `util/utils.go:115-123` (`GetPkgUrl`).

**Flag coverage today:** `--pkg-url` exists on push_generic/maven/go/conda/composer/rpm/cargo/npm/
dart/python/nuget (+ `pull.go:166`, `migrate_cmd.go:123`, `configure_npm.go:199`). **Missing:**
terraform, debian, conan, swift, puppet.

**Work items:**

- [ ] **W1. Derive `PkgURL` when credentials are explicit** — in push PreRun (or a shared helper),
  when `--api-url` is set and `--pkg-url` is unset, resolve the package base from the API URL
  (call `GetSystemInfo` → `registryUrl`, same as `cmd/auth/login.go:318-347`), with the resolved
  (secret-free) endpoint printed at debug/info. This fixes every explicit-credential caller at once.
- [ ] **W2. Early URL validation** — before streaming bytes, print the resolved package endpoint
  (host + `/pkg/<account>/<registry>` prefix, no token) and fail fast if empty. npm/dart already
  hard-fail on empty (`push_npm.go:123-126`, `push_dart.go:114-116`) — generalize that check.
- [ ] **W3. Gate the deprecation** — do **not** remove `--pkg-url` until W1 ships; soften the
  deprecation message in `util/utils.go:115-123` to state it remains supported for explicit-credential
  flows, or drop the notice until the root fix lands.
- [ ] **W4. Terraform push — no flag parity (decided 2026-08-07).** Do **not** add `--pkg-url` to
  `push_terraform.go` or the other missing types (debian/conan/swift/puppet). Instead, document hc
  as **unsupported for Terraform bridge pushes** (explicit-credential path has no escape hatch) and
  keep `scripts/art-tf-provider-migrate.sh` as the supported path. With W1 landing, explicit-flag
  pushes for the *supported* types no longer need `--pkg-url` at all.
- [ ] **W5. A/B regression test** — scripted legs A/B/C from the issue register (explicit no-flag →
  must now succeed; explicit + `--pkg-url`; saved-config) against DEV with a disposable NuGet package.

**Acceptance:** leg A (explicit credentials, no `--pkg-url`) uploads successfully; the deprecation
notice is removed or gated; no push type lacks a URL override.

---

## 4. P2f — dry-run delete prompts; piped `y` performs a real delete

**Code map (verified):** `cmd/artifact/command/delete.go` — dry-run default `true` (137); on
`DryRun==true` response it prints impact then prompts
`Above %s will be soft deleted. Do you want to proceed? (y/N):` (250) and on `y` re-executes with
`dryRun=false` (265). No `--yes` flag; no post-delete verification. Force prompt at 65-81.

**Work items:**

- [ ] **W1. Dry-run never mutates** — when `--dry-run=true`, print the impact preview and exit 0
  with **no prompt and no follow-up call**. Non-TTY stdin must be a clean preview, not an abort.
- [ ] **W2. Explicit execution mode** — real deletes require `--dry-run=false` (already the flag
  semantic; keep) **and** interactively prompt only when stdin is a TTY; otherwise fail with
  "confirmation required".
- [ ] **W3. `--yes` flag** — non-interactive confirmation for real deletes (`--dry-run=false --yes`).
  Refuse `--yes` together with `--dry-run=true` (contradiction → error).
- [ ] **W4. Machine-readable guarantee** — dry-run output includes `"mutated": false` (or similar)
  so wrappers can assert the no-mutation contract.
- [ ] **W5. Regression tests** — piped `y` + `--dry-run=true` ⇒ no mutation; closed stdin ⇒ clean
  exit 0 preview; `--dry-run=false --yes` ⇒ real soft delete.

**Acceptance:** the documented trap (`printf 'y\n' | ... --dry-run=true`) becomes impossible;
preview is safe and automation-friendly.

---

## 5. P2c — Conda/Terraform delete reports success while retaining the package

**Context:** single bulk-delete path for all types
(`cmd/artifact/command/delete.go:144-169` → `BulkDeleteArtifactsWithResponse`); success is inferred
from HTTP 200 + parsed counts, never re-verified. The server-side delete defect for Conda/Terraform
is a **platform** issue (`harness-har-platform-open-issues.md`); provider-file DELETE is 405.

**Work items (CLI-side):**

- [ ] **W1. Post-delete verification** — after a real delete, re-query the coordinate (type-specific
  existence read) and return non-zero with a distinct `UNCHANGED` result if it persists. Where no
  cheap existence read exists, at minimum surface the server's per-coordinate status instead of a
  blanket success line.
- [ ] **W2. Distinguish outcomes** — report `SOFT_DELETED` / `HARD_DELETED` / `UNCHANGED` /
  `UNSUPPORTED` per coordinate instead of one success count.
- [ ] **W3. Fail fast on unsupported types** — if the platform marks Conda/Terraform hard delete as
  unsupported (405), detect and fail **before** claiming success, with a pointer to the REST
  restore path for soft deletes.
- [ ] **W4. Coordination** — track the platform delete defect; CLI work here is guardrails, the
  actual Conda/Terraform deletion fix is server-side. **Decided (2026-08-07): ship the CLI
  verification ahead of the platform fix** — the defect becoming loud (non-zero exit + `UNCHANGED`)
  is desirable for the bridge; the platform fix is figured out separately.

**Acceptance:** a delete that changes nothing exits non-zero and says so; supported deletes verify.

---

## 6. P2d — include/exclude patterns (residual work)

**Already done (verified):** patterns are applied — file-level
(`migratable/registry.go:223-231`, types per `util/patternUtil.go:146-152`) and package-level
(`registry.go:297-304`, types per `patternUtil.go:155-162`); mutual exclusion enforced at
`registry.go:128-131`. The `// NOT IMPLEMENTED YET` comment at `types/config.go:108-109` is stale.

**Remaining work:**

- [ ] **W1. Reject unsupported configs** — at `validateConfig`, if `includePatterns`/`excludePatterns`
  are set for a type that is neither file-level nor package-level filterable (e.g. DEBIAN,
  TERRAFORM, PUPPET), fail with an explicit error instead of silently ignoring. Scope controls must
  never be no-ops.
- [ ] **W2. Fix the stale comment + docs** — `types/config.go:108-109` and the migration config
  docs; document which types support which granularity (mirror the `--help` granularity table at
  `migrate_cmd.go:97-103`).
- [ ] **W3. Per-type filter tests** — mock_jfrog integration tests proving include/exclude behavior
  for one file-level (GENERIC) and one package-level (CONDA or RPM) type, plus the W1 rejection.

**Acceptance:** unsupported filter fields are a config error; the steering doc's P2d entry can be
closed as fixed (with version noted).

---

## 7. P2e — date-filtered runs can under-migrate index-seeded types

**Assessment:** this is an **operational limitation by design** (the enumeration seed itself is
date-filtered; see `registry.go:177-221` and the PyPI/Conda/RPM seeding paths), not a bug with an
obvious code fix — a full no-filter run is the completeness path. The issue register already treats
it as an operational rule.

**Work items (UX/documentation only):**

- [ ] **W1. Warn at runtime** — when a `dateFilter` is set for PYTHON/CONDA/RPM (index-seeded /
  metadata-driven types), print a warning at `validateConfig` time mirroring the existing MAVEN
  warning (`config.go:191-199`): "date-filtered runs can omit in-scope content; a full run with
  overwrite:false is the completeness path".
- [ ] **W2. Doc cross-reference** — link the warning/README to the operational rule and
  `scripts/art-touch-rpm-seeds.sh` workaround.

**Acceptance:** operators cannot set a date filter on an index-seeded type without seeing the
limitation.

---

## 8. P3c — verbose output dumps the entire source listing as one line

**Code map (verified):** `adapter/jfrog/adapter.go:1391-1398` (`Get files from registry: %v` —
full `[]types.File` dump) and `:1401-1408` (`SearchFiles` dump). The registry layer already logs a
bounded count (`registry.go:142-143`).

**Work items:**

- [ ] **W1. Bound the lines** — replace the `%v` dumps with count + first-N sampled paths (e.g. 20).
- [ ] **W2. Opt-in full listing** — full enumeration goes to a JSON-lines file or a dedicated
  `--verbose-files` flag, never one log line.
- [ ] **W3. Redaction-friendly format** — one record per line so logs can be filtered/redacted.

**Acceptance:** `-v` on a full repository is log-safe; full listing available via explicit opt-in.

---

## 9. P3b — registry create stub; registry get JSON `[]`; artifact list ignores registry

**Code map (verified):**
- `cmd/registry/command/create.go:14-28` — `RunE` returns "not yet implemented".
- `cmd/registry/command/get.go:18-74` — search-based list; empty result prints `[]`.
- `cmd/artifact/command/list.go:19-72` — no positional args; `hc artifact list myreg` silently runs
  account-wide (registry only via `--registry` flag).

**Work items:**

- [ ] **W1. `registry get`** — implement a true single-registry GET (by identifier) so a missing
  registry is an explicit error, not `[]`; add JSON/table parity tests.
- [ ] **W2. `artifact list`** — either accept a positional registry arg (validated via cobra `Args`)
  or error on unexpected positional args; never silently widen scope.
- [ ] **W3. `registry create`** — decide: implement against the registry-create REST surface, or
  keep the stub but make the error message explicit that REST is the supported path (current
  behavior already fails, so this is polish, lowest priority).

**Acceptance:** no command returns an ambiguous empty/overbroad result; tests prove registry scoping.

---

## 10. P3a — TERRAFORM/RUBY capability gaps

**Verified status:** TERRAFORM **is** implemented in-tree (enum `types/config.go:45`; jfrog
`GetPackages` `adapter/jfrog/adapter.go:628-663`; version filters `version.go:125-127,166-185`;
file upload `file.go:177-213`; HAR `uploadTerraformFile`). Gaps: `--help` omits it (fixed by §1-W1),
and the register notes it needs network-mirror semantics (all platform files per provider version)
— **validate the in-tree implementation against that requirement before claiming support**. RUBY is
absent everywhere.

**Work items:**

- [ ] **W1. TERRAFORM validation** — test the in-tree TERRAFORM path against the network-mirror
  completeness requirement (all platform files per version); fix or document the gap. Update
  `indexApplicable`/filter helpers if needed.
- [ ] **W2. RUBY** — defer unless the bridge must replicate Ruby (currently proxy-only, no local
  content). If undertaken, the full add-a-type checklist applies: enum + `knownArtifactTypes` +
  help text; jfrog `GetPackages`/`GetVersions` branches; `package.go` branch; `version.go` filters;
  `file.go` upload body; `adapter/har` upload; `patternUtil`/`selector` helpers; `indexApplicable`.
- [ ] **W3. Explicit unsupported failure** — already covered by §1 validation (unknown types fail
  loudly); RUBY will get this for free until implemented.

**Acceptance:** TERRAFORM either verified-complete or documented as mirror-script-only; RUBY fails
explicitly until implemented.

---

## Cross-cutting

- **Tests:** extend `adapter/mock_jfrog` integration harness for the migration-side items
  (§1, §2, §6); scripted DEV A/B tests for push/delete (§3, §4, §5) using the disposable-package
  conventions already in the register.
- **Docs:** after each PR lands, update `harness-hc-cli-open-issues_md.md` statuses (move fixed
  items to "Previously closed" with version + date), and refresh `CLAUDE.md` where behavior changed
  (it currently pre-dates the artifactType validation and pattern filtering).
- **Sequencing note:** §1-W2 and §5 depend on §2 (exit-code plumbing); §3-W3 (deprecation gating)
  should land in the same change as §3-W1.

## Decisions (2026-08-07)

1. **Exit codes:** no opt-out flag. A migration with any failure exits non-zero, always. The bridge
   reconciles via the result file / read APIs instead of relying on exit-0-with-failures.
2. **Terraform push:** no `--pkg-url` parity. Document hc as unsupported for Terraform bridge
   pushes; the native mirror script stays the supported path.
3. **Delete verification:** ship the CLI post-delete verification ahead of the platform
   Conda/Terraform fix; the platform side is tracked separately.
4. **Process:** implement directly on `main` in the working tree — **no commits**; branching, JIRA
   tickets, and PR strategy to be figured out later. (Note: this deviates from the repo convention
   of feature-branch + PR; acceptable because nothing is committed or pushed.)
