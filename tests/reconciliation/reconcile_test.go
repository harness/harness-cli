//go:build e2e

// Package reconciliation contains end-to-end tests that focus on the
// reconciliation gate itself: version-string fidelity, per-version file
// membership, and the harness's ability to detect an artifact that is absent
// (so a migration that prints Success but uploaded nothing is caught).
package reconciliation

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestRpmVersionFormatNotBareSemver verifies HAR indexes the RPM version as the
// full version-release.arch string (1.0.0-1.x86_64) and NOT the bare semver
// (1.0.0). The negative assertion guards against a regression that would strip
// the release/arch.
func TestRpmVersionFormatNotBareSemver(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:   "RPM",
		PackageType:    "RPM",
		SourceRegistry: "rpm-local",
		DestRegistry:   harness.UniqueRegistry(t, "reconrpm"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "mockpkg", Version: "1.0.0-1.x86_64"},
		},
		NotExpectedFiles: []harness.ExpectedFile{
			{Pkg: "mockpkg", Version: "1.0.0"},
		},
	})
}

// TestPerVersionFileMembership verifies not just that a version exists but that
// the expected file is present in the destination's per-version file listing
// (GetAllFilesForVersion) — the stricter membership check the migration's skip
// logic relies on.
func TestPerVersionFileMembership(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "MAVEN",
		PackageType:    "MAVEN",
		SourceRegistry: "maven-local",
		DestRegistry:   harness.UniqueRegistry(t, "reconmaven"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "com.example:sample", Version: "1.0", FileName: "sample-1.0.jar"},
		},
	})
}

// TestReconcileDetectsAbsence provisions an empty destination registry (no
// migration is run) and asserts the negative reconcile primitive correctly
// reports the artifacts as absent. This proves the gate can catch a migration
// that claimed success but uploaded nothing.
func TestReconcileDetectsAbsence(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t)

	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)

	spec := harness.Spec{
		ArtifactType:   "GENERIC",
		PackageType:    "GENERIC",
		SourceRegistry: "generic-local",
		DestRegistry:   harness.UniqueRegistry(t, "reconabsent"),
		NotExpectedRawURIs: []string{
			"default/default/data/config.json",
			"default/default/bin/run.sh",
		},
	}

	ref := harness.CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)
	t.Cleanup(func() { harness.DeleteRegistry(t, creds, ref) })

	// No migration performed: everything must be reported absent.
	harness.ReconcileAbsent(t, creds, spec)
}
