//go:build e2e

// Package raw contains the end-to-end migration test for RAW (generic file)
// artifacts: MOCK_JFROG raw-local -> HAR GENERIC registry.
package raw

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateRAW migrates raw-local files whose paths satisfy HAR's generic-file
// layout (package/version/file, i.e. at least two slashes) and reconciles each
// landed URI with a HEAD existence check. Shallow paths such as docs/readme.txt
// are excluded because the server rejects them with a 400 today.
func TestMigrateRAW(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC", // uploadRawFile uses the generic-file endpoints
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "raw"),
		IncludePatterns:   []string{"configs/**", "assets/**"},
		ExpectedRawURIs: []string{
			"configs/v1/config.yaml",
			"assets/images/logo.png",
		},
	})
}
