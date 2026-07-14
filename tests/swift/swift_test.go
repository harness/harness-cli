//go:build e2e

// Package swift contains the end-to-end migration test for Swift artifacts:
// MOCK_JFROG swift-local -> HAR SWIFT registry.
package swift

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateSwift migrates the "myscope.harness" Swift package (both versions)
// and verifies each version is present in the destination. Swift is
// package-level filterable, and the package name is derived from the source URI
// layout as <scope>.<name>.
func TestMigrateSwift(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "SWIFT",
		PackageType:     "SWIFT",
		SourceRegistry:  "swift-local",
		DestRegistry:    harness.UniqueRegistry(t, "swift"),
		IncludePatterns: []string{"myscope.harness"},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "myscope.harness", Version: "1.0.0"},
			{Pkg: "myscope.harness", Version: "1.0.1"},
		},
	})
}
