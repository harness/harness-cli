//go:build e2e

package raw

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateRAWShallowRejected migrates the full raw-local repository (no
// include filter) and asserts the path-layout contract: files with two or more
// path segments land, while a shallow one-segment path (docs/readme.txt) is
// rejected by HAR's generic-file rules (400) and never appears in the
// destination. The CLI still exits 0 on the per-file failure; the negative
// reconcile is what proves the shallow file was not migrated.
func TestMigrateRAWShallowRejected(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:   "RAW",
		PackageType:    "GENERIC",
		SourceRegistry: "raw-local",
		DestRegistry:   harness.UniqueRegistry(t, "rawshallow"),
		// Deep paths (2+ segments) must land.
		ExpectedRawURIs: []string{
			"configs/v1/config.yaml",
			"assets/images/logo.png",
		},
		// Shallow path (1 segment) must be rejected and absent.
		NotExpectedRawURIs: []string{
			"docs/readme.txt",
		},
	})
}
