//go:build e2e

// Package idempotency contains end-to-end tests that run a migration twice and
// assert the re-run behaves correctly: with overwrite disabled, already-migrated
// artifacts are skipped; with overwrite enabled, they are re-processed. Both
// modes must leave the destination intact (verified by a positive reconcile
// after each run).
package idempotency

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestRawIdempotentSkip migrates a RAW file, then re-runs. With overwrite=false
// the second run must skip the already-present file (HEAD 200 short-circuit).
func TestRawIdempotentSkip(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t)

	harness.RunIdempotent(t, creds, harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC",
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "idemraw"),
		IncludePatterns: []string{"configs/**"},
		ExpectedRawURIs: []string{"configs/v1/config.yaml"},
	})
}

// TestVersionedIdempotentSkip migrates a multi-version Python package, then
// re-runs. With overwrite=false the second run must skip existing versions
// (VersionExists short-circuit for non-Maven/NPM types).
func TestVersionedIdempotentSkip(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t)

	harness.RunIdempotent(t, creds, harness.Spec{
		ArtifactType:   "PYTHON",
		PackageType:    "PYTHON",
		SourceRegistry: "python-local",
		DestRegistry:   harness.UniqueRegistry(t, "idempython"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "requests", Version: "2.28.0"},
			{Pkg: "requests", Version: "2.29.0"},
		},
	})
}

// TestOverwriteReplaces re-runs a RAW migration with overwrite=true. The second
// run must re-process (not skip) without failures, and the file must remain
// present.
func TestOverwriteReplaces(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t)

	harness.RunIdempotent(t, creds, harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC",
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "idemoverwrite"),
		IncludePatterns: []string{"configs/**"},
		ExpectedRawURIs: []string{"configs/v1/config.yaml"},
		Overwrite:       true,
	})
}
