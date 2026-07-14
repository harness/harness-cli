//go:build e2e

// Package generic contains the end-to-end migration test for the GENERIC
// artifact type: MOCK_JFROG generic-local -> HAR GENERIC registry.
package generic

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateGeneric migrates the generic-local files. GENERIC uploads land at
// default/default/<uri> on the generic-file endpoint; reconciliation HEADs those
// paths directly (see reconcileGeneric).
func TestMigrateGeneric(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "GENERIC",
		PackageType:    "GENERIC",
		SourceRegistry: "generic-local",
		DestRegistry:   harness.UniqueRegistry(t, "generic"),
	})
}
