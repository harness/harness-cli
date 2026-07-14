//go:build e2e

// Package helmhttp contains the end-to-end migration test for Helm charts
// served over an HTTP (index.yaml) JFrog repo: MOCK_JFROG helm-http-local ->
// HAR HELM registry.
package helmhttp

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateHelmHTTP migrates the nested ChartA/ChartB/abc chart from the
// helm-http source into a GENERIC destination registry and verifies the chart
// archive landed at its source-relative upload path.
//
// HELM_HTTP migration uploads charts via the generic-file endpoint (not OCI),
// and flat single-segment chart names fail HAR's package/version/file path
// rules. The nested fixture produces ChartA/ChartB/abc-1.0.1.tgz (three path
// segments), which uploads and reconciles reliably without production changes.
func TestMigrateHelmHTTP(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "HELM_HTTP",
		PackageType:     "GENERIC",
		SourceRegistry:  "helm-http-local",
		DestRegistry:    harness.UniqueRegistry(t, "helmhttp"),
		IncludePatterns: []string{"ChartA/**"},
		ExpectedRawURIs: []string{"ChartA/ChartB/abc-1.0.1.tgz"},
	})
}
