//go:build e2e

// Package rpm contains the end-to-end migration test for RPM artifacts:
// MOCK_JFROG rpm-local -> HAR RPM registry.
package rpm

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateRPM migrates the mockpkg RPM (enumerated via repodata/primary.xml)
// and verifies the parsed NEVRA is present in the destination. HAR indexes RPM
// versions as version-release.arch (e.g. 1.0.0-1.x86_64), not the bare semver.
func TestMigrateRPM(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "RPM",
		PackageType:    "RPM",
		SourceRegistry: "rpm-local",
		DestRegistry:   harness.UniqueRegistry(t, "rpm"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "mockpkg", Version: "1.0.0-1.x86_64"},
		},
	})
}

// TestMigrateRPMValidateTicket1 runs an in-process RPM migration against the
// project in HARNESS_PROJECT_ID (e.g. arham_test_proj), logs per-file transfer
// stats for ticket-1 validation (enumerated size vs HAR outcome), and reconciles
// that mockpkg@1.0.0-1.x86_64 landed in the destination.
//
// The destination registry is left in HAR after the test (no cleanup) so you can
// inspect it in the Harness UI.
//
// Run after exporting QA creds:
//
//	go test -tags=e2e ./tests/rpm/ -run TestMigrateRPMValidateTicket1 -v -count=1
func TestMigrateRPMValidateTicket1(t *testing.T) {
	creds := harness.RequireEnv(t)
	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)

	spec := harness.Spec{
		ArtifactType:   "RPM",
		PackageType:    "RPM",
		SourceRegistry: "rpm-local",
		DestRegistry:   harness.UniqueRegistry(t, "rpm_validate"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "mockpkg", Version: "1.0.0-1.x86_64"},
		},
	}

	ref := harness.CreateRegistry(t, creds, spec.DestRegistry, spec.PackageType)

	t.Logf("validation run: project=%s/%s dest_registry=%s registry_ref=%s",
		creds.OrgID, creds.ProjectID, spec.DestRegistry, ref)
	t.Logf("registry left in HAR for manual inspection (not deleted after test)")

	stats := harness.MigrateInProcessStats(t, creds, spec)
	for _, fs := range stats.FileStats {
		t.Logf("  file stat: name=%q uri=%q size=%d status=%s err=%q",
			fs.Name, fs.Uri, fs.Size, fs.Status, fs.Error)
	}
	if len(stats.FileStats) == 0 {
		t.Fatal("expected at least one file stat from RPM migration")
	}

	// Ticket 1 symptom: enumeration sets pkg.Size from <size package="0"/> in
	// primary.xml, so CLI stats show 0 even though the RPM artifact is non-empty.
	if stats.FileStats[0].Size == 0 {
		t.Logf("TICKET-1 OBSERVED: FileStat.Size is 0 (CLI will show 0.00B); HAR should still index NEVRA from RPM bytes")
	}

	harness.Reconcile(t, creds, spec)
}
