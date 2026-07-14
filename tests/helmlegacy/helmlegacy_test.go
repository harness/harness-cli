//go:build e2e

// Package helmlegacy contains the end-to-end migration test for legacy Helm
// charts enumerated from an index.yaml: MOCK_JFROG helm-legacy-local -> HAR HELM
// registry (charts are pushed as OCI artifacts).
package helmlegacy

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateHelmLegacy migrates the nginx chart declared in the source
// index.yaml and verifies the chart version is present in the destination HELM
// registry. The destination name/version come from the chart's Chart.yaml
// (nginx / 8.2.0).
func TestMigrateHelmLegacy(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "HELM_LEGACY",
		PackageType:    "HELM",
		SourceRegistry: "helm-legacy-local",
		DestRegistry:   harness.UniqueRegistry(t, "helmlegacy"),
		ExpectedTags: []harness.ExpectedTag{
			{Image: "nginx", Tag: "8.2.0"},
		},
	})
}
