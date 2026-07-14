//go:build e2e

// Package npm contains the end-to-end migration test for NPM artifacts:
// MOCK_JFROG npm-local -> HAR NPM registry.
package npm

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateNPM migrates a single, deterministic NPM package (lodash) and
// verifies the version is present in the destination. The include pattern keeps
// the migration narrow so the assertion is stable regardless of how many
// versions the source metadata advertises. The destination version is derived
// by the migration from the tarball's package.json, which equals the tarball's
// file-name version for this fixture (4.17.21-alpha.0).
func TestMigrateNPM(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "NPM",
		PackageType:     "NPM",
		SourceRegistry:  "npm-local",
		DestRegistry:    harness.UniqueRegistry(t, "npm"),
		IncludePatterns: []string{"lodash/**"},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "lodash", Version: "4.17.21-alpha.0"},
		},
	})
}
