//go:build e2e

// Package enumeration contains end-to-end tests for source enumeration edge
// cases: hybrid index+tree discovery, multi-reference packages, and empty
// repositories. They assert enumeration surfaces exactly the artifacts expected
// (no missing, no duplicated, no phantom entries).
package enumeration

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestHelmHTTPTreeOnlyChartRecovered verifies the hybrid HELM_HTTP enumeration
// recovers a chart that exists on disk (tree sweep) but is NOT listed in
// index.yaml. The source index only advertises nginx; ChartA/ChartB/abc is
// discovered solely by the tree sweep and must still migrate.
func TestHelmHTTPTreeOnlyChartRecovered(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "HELM_HTTP",
		PackageType:     "GENERIC",
		SourceRegistry:  "helm-http-local",
		DestRegistry:    harness.UniqueRegistry(t, "enumhelmtree"),
		IncludePatterns: []string{"ChartA/**"},
		ExpectedRawURIs: []string{"ChartA/ChartB/abc-1.0.1.tgz"},
	})
}

// TestEmptyRegistryZeroFiles verifies that enumerating an empty source
// repository yields zero migrated files (no phantom default file) and nothing
// lands in the destination.
func TestEmptyRegistryZeroFiles(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t)

	harness.RunExpectZeroFiles(t, creds, harness.Spec{
		ArtifactType:   "RAW",
		PackageType:    "GENERIC",
		SourceRegistry: "empty-local",
		DestRegistry:   harness.UniqueRegistry(t, "enumempty"),
	})
}
