//go:build e2e

// Package failure contains end-to-end tests for per-file failure handling. They
// run the migration in-process so the raw per-file TransferStats are visible —
// the subprocess CLI exits 0 even on per-file failures, so only the in-process
// stats can prove a specific file was recorded as Failed while its siblings
// succeeded.
package failure

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/tests/harness"
)

// provisionGeneric creates a GENERIC destination for a failure spec and returns
// the spec (with a unique registry) after scheduling cleanup.
func provisionGeneric(t *testing.T, creds harness.Creds, prefix, source string) harness.Spec {
	t.Helper()
	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)
	spec := harness.Spec{
		ArtifactType:   "RAW",
		PackageType:    "GENERIC",
		SourceRegistry: source,
		DestRegistry:   harness.UniqueRegistry(t, prefix),
	}
	ref := harness.CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Cleanup(func() { harness.DeleteRegistry(t, creds, ref) })
	return spec
}

// TestRawShallowPathFailsButRestSucceeds migrates the whole raw-local repo and
// asserts the mixed outcome: the deep config file succeeds while the shallow
// docs/readme.txt is recorded as Failed (rejected by HAR's generic-file rules).
func TestRawShallowPathFailsButRestSucceeds(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t)

	spec := provisionGeneric(t, creds, "failshallow", "raw-local")
	stats := harness.MigrateInProcessStats(t, creds, spec)

	if got, ok := harness.FileStatByURI(stats, "docs/readme.txt"); !ok {
		t.Errorf("expected a file stat for the shallow path docs/readme.txt")
	} else if got.Status != types.StatusFail {
		t.Errorf("shallow path docs/readme.txt: expected Failed, got %s (err=%q)", got.Status, got.Error)
	}

	if got, ok := harness.FileStatByURI(stats, "configs/v1/config.yaml"); !ok {
		t.Errorf("expected a file stat for the deep path configs/v1/config.yaml")
	} else if got.Status != types.StatusSuccess {
		t.Errorf("deep path configs/v1/config.yaml: expected Success, got %s (err=%q)", got.Status, got.Error)
	}
}
