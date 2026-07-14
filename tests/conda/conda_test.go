//go:build e2e

// Package conda contains the end-to-end migration test for Conda artifacts:
// MOCK_JFROG conda-local -> HAR CONDA registry.
package conda

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateConda migrates the mockpkg conda package (enumerated via
// repodata.json) and verifies the package version is present in the destination.
// Conda derives name/version/subdir from repodata.json; the destination version
// is expected to be the plain package version (subdir is a separate attribute).
func TestMigrateConda(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "CONDA",
		PackageType:    "CONDA",
		SourceRegistry: "conda-local",
		DestRegistry:   harness.UniqueRegistry(t, "conda"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "mockpkg", Version: "1.2.0"},
		},
	})
}
