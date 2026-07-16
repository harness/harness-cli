//go:build e2e

package npm

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateNPMScoped migrates the scoped package @har/sample-package and
// verifies the scoped name is preserved in the destination. The include pattern
// scopes enumeration to the @har namespace so the assertion is deterministic.
func TestMigrateNPMScoped(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "NPM",
		PackageType:     "NPM",
		SourceRegistry:  "npm-local",
		DestRegistry:    harness.UniqueRegistry(t, "npmscoped"),
		IncludePatterns: []string{"@har/**"},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "@har/sample-package", Version: "1.0.0"},
		},
	})
}
