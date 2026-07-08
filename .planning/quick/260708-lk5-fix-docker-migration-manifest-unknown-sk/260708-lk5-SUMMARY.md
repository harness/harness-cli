---
quick_id: 260708-lk5
status: complete
date: 2026-07-08
commit: 7d1055a
branch: fix/AH-docker-migration-manifest-unknown
---

# Summary — Fix Docker migration MANIFEST_UNKNOWN skip-image bug

## Root cause

`Package.Migrate()` (DOCKER/HELM branch, `module/ar/migrate/migratable/package.go`)
copied images with `crane.CopyRepository(srcImage, dstImage, ...)`. In
go-containerregistry v0.20.6 that helper fans every tag out into a single
`errgroup`. The first tag whose `puller.Get` fails — e.g. a stale JFrog tag folder
whose `list.manifest.json` references a `linux/amd64` platform-manifest digest that
was garbage-collected when a newer `-cgr` image was pushed, so the registry returns
`MANIFEST_UNKNOWN` — cancels the shared context, aborts every remaining tag, and
`g.Wait()` returns that one error. The whole image was therefore recorded as a
single `StatusFail`, matching the reported "skips all remaining tags and marks the
entire image failed".

## Change

Replaced the single `CopyRepository` call with a new `Package.migrateOCI` method
that copies **one tag at a time** via `crane.Copy`:

- `crane.ListTags(srcImage)` enumerates tags; a listing failure is a genuine
  image-level `StatusFail` (nothing to iterate).
- No-clobber preserved: when `!Overwrite`, destination tags are pre-listed once and
  already-present tags are skipped (Pre() already records them as Skipped, so no
  double count).
- Per tag: success → `StatusSuccess`; stale/orphaned **source** manifest →
  `StatusSkip` (loop continues); any other error → `StatusFail` (loop continues).
- `isStaleSourceManifestErr(err)` classifies `MANIFEST_UNKNOWN`,
  `MANIFEST_BLOB_UNKNOWN`, `BLOB_UNKNOWN`, `NAME_UNKNOWN`, and bare HTTP 404 as
  stale-source; auth/quota/network/destination-push errors stay genuine failures.
- Emits a per-image `migrated / skipped / failed / total` summary line.

One orphaned tag can no longer take down the rest of an image, and the migration
summary now shows exactly which tags moved, which were skipped, and why.

## Files

- `module/ar/migrate/migratable/package.go` — imports (`net/http`, `transport`);
  DOCKER/HELM branch now delegates to `migrateOCI`; added `migrateOCI` +
  `isStaleSourceManifestErr`.
- `module/ar/migrate/migratable/package_test.go` — `TestIsStaleSourceManifestErr`
  (11 table cases: MANIFEST_UNKNOWN, blob/name unknown, bare 404, wrapped error,
  auth/denied/rate-limit genuine failures, non-transport error, nil).

## Verification

- `go build ./module/ar/migrate/...` — clean.
- `go test ./module/ar/migrate/...` — all pass.
- `go vet ./module/ar/migrate/migratable/...` — clean.

Commit `7d1055a` on branch `fix/AH-docker-migration-manifest-unknown`.

## Follow-ups (not done)

- No integration test drives `migrateOCI` end-to-end against a fake registry (the
  existing suite has no OCI HTTP harness). The per-tag skip/continue behaviour is
  covered indirectly via the classifier unit test; an httptest-backed registry test
  could be added later for full-loop coverage.
