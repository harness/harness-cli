//go:build e2e

// Package debian contains the end-to-end migration test for Debian artifacts:
// MOCK_JFROG debian-local -> HAR DEBIAN registry.
package debian

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateDebian migrates the debian-local repository and verifies the nginx
// binary package version is present in the destination.
//
// NOTE: "DEBIAN" must be an accepted registry packageType in the target
// environment. If the environment does not support Debian registries, the
// CreateRegistry step fails fast with the server's enum error — that is the
// signal to enable Debian support (or gate this test) rather than a migration
// defect.
func TestMigrateDebian(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "DEBIAN",
		PackageType:    "DEBIAN",
		SourceRegistry: "debian-local",
		DestRegistry:   harness.UniqueRegistry(t, "debian"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "nginx", Version: "1.18.0-1"},
		},
	})
}
