//go:build e2e

// Package dart contains the end-to-end migration test for Dart (pub) artifacts:
// MOCK_JFROG dart-local -> HAR DART registry.
package dart

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateDart migrates the sample_dart_pkg package (both versions declared
// in the source pub metadata) and verifies each version is present in the
// destination. The include pattern scopes enumeration to a single package that
// has full metadata, keeping the assertion deterministic.
func TestMigrateDart(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "DART",
		PackageType:     "DART",
		SourceRegistry:  "dart-local",
		DestRegistry:    harness.UniqueRegistry(t, "dart"),
		IncludePatterns: []string{"packages/sample_dart_pkg/**"},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "sample_dart_pkg", Version: "1.0.0"},
			{Pkg: "sample_dart_pkg", Version: "1.1.0"},
		},
	})
}
