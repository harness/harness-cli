# Firewall audit/explain: migrate to sync bulk-evaluate API

**Status:** approved
**JIRA:** AH-4900 (server), harness-cli follow-up (this spec)
**Author:** Sourabh
**Date:** 2026-08-05

## Summary

Move `hc registry fw audit` and `hc registry fw explain` off the async
`POST /api/v3/scans/bulk-evaluate` + poll flow onto the new synchronous
`POST /api/v3/scans/bulk-evaluate-sync` endpoint. Batch requests in parallel
through a worker pool to keep large lock files interactive. Keep the old
async path behind an `--async` flag. The user-facing response shape (text
tables and JSON) stays identical.

## Motivation

The current flow is: initiate → poll every 2s → repeat per batch, serialized.
For a 400-package lock file at 50/batch, that is 8 batches × (≥1 poll cycle
+ server processing) done sequentially — dominated by polling latency, not
the actual evaluation cost.

The sync API returns results in one call with no DB writes. Running batches
concurrently gives near-linear speedup and lets us stream per-batch progress
to the terminal as work completes.

## Scope

**In scope**
- `cmd/registry/command/fw_audit.go` — batch evaluator pluggable between
  sync and async; parallel batch execution; per-batch progress lines.
- `cmd/registry/command/fw_explain.go` — single-artifact sync call; reuse
  `displayScanDetails` on the sync response body.
- `api/ar_v3/openapi.yaml` — add the `POST /scans/bulk-evaluate-sync`
  operation and its request/response schemas; regenerate `internal/api/ar_v3/
  client_gen.go` via `make generate`.

**Out of scope**
- Changes to any other `hc` subcommand.
- Server-side changes (AH-4900 already shipped in the `artifact-registry`
  repo).
- Firewall exception / build-info workflows.

## Design

### CLI surface

`fw audit`:
```
hc registry fw audit --registry <name> --file <path>
    [--async]                # opt into old async initiate+poll flow
    [--batch-size 20]        # sync mode only; async is hardcoded to 50
    [--workers 10]           # concurrent batches; sync mode only
    [--org] [--project]
```

`fw explain`:
```
hc registry fw explain --registry <name> --package <name> --version <v>
    [--async]                # opt into old async initiate+poll flow
    [--org] [--project]
```

Defaults: sync path, `--batch-size 20`, `--workers 10`. `skipCache` is
always sent as `true` server-side; not exposed as a flag.

### Architecture

**Evaluator abstraction** (new file: `cmd/registry/command/fw_evaluator.go`)

```go
type Evaluator interface {
    Evaluate(ctx context.Context, batch []Dependency, info batchInfo) ([]ScanResult, error)
    // async mode enforces server max
    MaxBatchSize() int
}
```

Two implementations:

- `asyncEvaluator` — wraps the existing `initiateBatchEvaluation` +
  `pollBatchEvaluation` + `extractScanResults` sequence. Behavior unchanged.
  `MaxBatchSize() = 50`.
- `syncEvaluator` — one call to the new `BulkScanEvaluateSyncWithResponse`.
  Sends `skipCache: true`. `MaxBatchSize() = 50` (server cap). Returns a
  `ScanResult` per artifact in the request, preserving order.

`ScanResult` gets an optional field for detail rendering used by
`fw explain`:
```go
type ScanResult struct {
    PackageName             string
    Version                 string
    ScanID                  string
    ScanStatus              string
    PolicySetFailureDetails *ar_v3.ArtifactScanDetails // sync only; nil for async
}
```
The audit output (text/JSON) does not print `PolicySetFailureDetails`, so
the field is inert there.

**Batch runner** (in `fw_audit.go`)

Replaces the current sequential `processBatches`:

1. Slice `dependencies` into batches of `min(--batch-size, evaluator.MaxBatchSize())`.
2. `errgroup.WithContext(...).SetLimit(workers)`. Async mode forces
   `workers = 1` regardless of flag (preserves current behavior).
3. Each goroutine: `results, err := evaluator.Evaluate(ctx, batch, info)`.
   - On success: push `batchOutcome{idx, results, nil}` onto a buffered
     results channel.
   - On error: build a `[]ScanResult` where every dep gets
     `ScanStatus: "UNKNOWN"` and `ScanID: ""`; push `batchOutcome{idx,
     unknowns, err}`. Do not cancel the group.
4. A single printer goroutine drains the channel:
   - Prints `✓ Batch N/M done — K pkgs (X BLOCKED, Y WARN, Z ALLOWED, W UNKNOWN)`
     as each batch completes.
   - Every 5 completions (and always the last), prints running totals
     line: `progress: n/m batches, k/total pkgs, X BLOCKED so far`.
   - Aggregates all results into a single slice returned to the caller.
5. Any batch with `err != nil` sets `hadFailures = true`. After the run,
   `displayResults` is invoked normally; the command returns exit 1 if
   `hadFailures`, else 0.

**Failure semantics**

- Network error, non-2xx status, or malformed body from the sync call
  → whole batch's deps flagged `UNKNOWN`, logged with `zerolog.Error`,
  execution continues.
- Server returns per-artifact `scanStatus: "UNKNOWN"` → passed through
  unchanged.
- Timeout: sync path uses the client's default HTTP timeout (no
  custom poll loop). Async path unchanged.

### `fw explain` sync path

1. Registry lookup — unchanged.
2. Build a one-element `[]ArtifactScanInput`; call
   `BulkScanEvaluateSyncWithResponse` with `skipCache: true`.
3. Read `resp.JSON200.Data[0]`. Extract `scanStatus`, `scanId` (may be
   the zero-UUID string), and the embedded `ArtifactScanDetails`.
4. Print the same "Scan Result" header block as today:
   ```
   Package:            <name>
   Version:            <v>
   Evaluation Status:  BLOCKED|WARN|ALLOWED|UNKNOWN
   Evaluation ID:      <scanId or zero-UUID>
   ```
5. Print the same "This artifact version is BLOCKED/WARN/ALLOWED"
   summary line via `p.Error / p.Step / p.Success`.
6. Call `displayScanDetails(&artifactScanDetails)` — the existing
   renderer works because the sync API returns the same
   `ArtifactScanDetails` shape (per AH-4900 spec:
   `policySetFailureDetails`, `publishedOn`, `packageAgeThreshold`,
   `AsSecurityPolicyFailureDetailConfig`, `AsLicensePolicyFailureDetailConfig`,
   `AsPackageAgeViolationPolicyFailureDetailConfig` are all present).
7. Drop the follow-up `GetArtifactScanDetailsWithResponse` call on the
   sync path — sync response is already the full detail body.

Async path (`--async`) keeps the current two-hop flow verbatim.

### OpenAPI update

Add to `api/ar_v3/openapi.yaml` under `paths:`:

```yaml
/scans/bulk-evaluate-sync:
  post:
    x-internal: false
    summary: Synchronous bulk evaluation
    description: Evaluates artifacts in-process, returns results immediately.
    operationId: BulkScanEvaluateSync
    tags:
      - Registry V3
    parameters:
      - $ref: "#/components/parameters/AccountIdentifier"
      - $ref: "#/components/parameters/OrgIdentifier"
      - $ref: "#/components/parameters/ProjectIdentifier"
    requestBody:
      $ref: "#/components/requestBodies/BulkScanEvaluateSyncRequest"
    responses:
      "200":
        $ref: "#/components/responses/BulkScanEvaluateSyncResponse"
      default:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/V3Error"
        description: Error response.
```

Add schemas:
- `BulkScanEvaluateSyncRequest` = same as `BulkScanEvaluationRequest` but
  with an optional `skipCache: boolean`.
- `BulkScanEvaluateSyncResponse` = `data: []ArtifactScanDetails` where
  each element carries `packageName`, `version`, `scanStatus`, `scanId`
  (UUID string, zero-UUID when no DB record), `registryId`,
  `registryName`, `parentRegistryId`, `parentRegistryName`, `packageType`,
  `purl`, and `policySetFailureDetails`.

Regenerate via `make generate`.

## Response-shape guarantee

**`fw audit` text output** — identical:
- Same header, same per-status count lines (Blocked / Warnings /
  Allowed / Unknown), same table columns
  (`Package Name`, `Version`, `Status`), same alphabetical sort.
- New per-batch progress lines are additive and use the same
  `progress.ConsoleReporter` styling.

**`fw audit` JSON output** — identical structure:
```json
[{"packageName": "...", "version": "...", "scanId": "...", "scanStatus": "..."}]
```
`scanId` is the zero-UUID string in sync mode (server returns no DB record).

**`fw explain` text output** — identical:
- Same "Scan Result" header, same field labels (`Evaluation Status`,
  `Evaluation ID`), same BLOCKED/WARN/ALLOWED summary line.
- `displayScanDetails` is reused verbatim; if the generated Go type for the
  sync response differs from `ar_v3.ArtifactScanDetails`, we add a thin
  adapter so `displayScanDetails` keeps its current signature and output
  stays byte-identical.
- `Evaluation ID` shows the zero-UUID in sync mode.

**`fw explain` JSON output** — identical field names.
`scanId` value: same zero-UUID caveat.

**Regression gate:** `--async` remains the ground truth. Any structural
diff between sync and async output (field names, ordering, labels) is
a bug to fix before merging.

## Testing

Extend `fw_audit_test.go`:
- Batcher: empty input, single batch, exact multiples, off-by-one
  remainder.
- `syncEvaluator` happy path: mocked `BulkScanEvaluateSyncWithResponse`
  returns per-artifact statuses; results propagate in request order.
- `syncEvaluator` failure path: mock returns network error; all deps in
  batch become `UNKNOWN`.
- Parallel run: three batches, one errors; final `[]ScanResult` has
  UNKNOWNs from the failed batch plus real results from the others;
  `hadFailures` = true.
- Per-artifact `UNKNOWN` from server passes through unchanged.

New `fw_explain_test.go` (or extend if present):
- Sync path renders BLOCKED with `policySetFailureDetails` covering all
  three category branches (Security, License, PackageAge).
- Sync path renders ALLOWED (no failure details) without crashing on
  nil fields.
- `--async` path unchanged (existing test coverage stays).

## Rollout / risk

- Behavior change is default-on but reversible with `--async` at any
  time. Users hitting a sync-API bug can opt out without a CLI rebuild.
- Server endpoint AH-4900 is already live (per the pasted spec).
  Failure mode is documented (UNKNOWN per artifact or per batch),
  never silent data loss.
- No DB or config schema changes.

## Open questions

None at spec time. Any generated-type surprises during the OpenAPI
regen (e.g., a distinct `ArtifactScanDetailsV3` type instead of reusing
`ArtifactScanDetails`) are handled by a small adapter in the explain
path — noted above.
