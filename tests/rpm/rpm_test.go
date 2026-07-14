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
