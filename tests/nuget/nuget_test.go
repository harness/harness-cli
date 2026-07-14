//go:build e2e

// Package nuget contains the end-to-end migration test for NuGet artifacts:
// MOCK_JFROG nuget-local -> HAR NUGET registry.
package nuget

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateNuGet migrates a single package version (company.grpc.pkg 1.0.0)
// and verifies both that the version exists and that its .nupkg is present in
// the destination's per-version file listing.
func TestMigrateNuGet(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "NUGET",
		PackageType:     "NUGET",
		SourceRegistry:  "nuget-local",
		DestRegistry:    harness.UniqueRegistry(t, "nuget"),
		IncludePatterns: []string{"foo/company.grpc.pkg/1.0.0/**"},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "company.grpc.pkg", Version: "1.0.0"},
		},
	})
}
