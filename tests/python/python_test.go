//go:build e2e

// Package python contains the end-to-end migration test for Python artifacts:
// MOCK_JFROG python-local -> HAR PYTHON registry.
package python

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigratePython migrates the python-local "requests" sdists and verifies
// both versions are present in the destination. The destination package
// name/version are read by the migration from each sdist's PKG-INFO, which the
// fixtures set to requests / 2.28.0 and 2.29.0.
func TestMigratePython(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "PYTHON",
		PackageType:    "PYTHON",
		SourceRegistry: "python-local",
		DestRegistry:   harness.UniqueRegistry(t, "python"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "requests", Version: "2.28.0"},
			{Pkg: "requests", Version: "2.29.0"},
		},
	})
}
