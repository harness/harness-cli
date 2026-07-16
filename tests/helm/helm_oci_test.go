//go:build e2e

// Package helm contains live end-to-end native HELM (OCI) migration tests. They
// migrate chart images from a real OCI source registry into HAR HELM and are
// gated on E2E_OCI_SOURCE_* — when unset, tests skip (see tests/ocismoke for
// offline copy mechanics).
package helm

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateHelmOCILive migrates a single chart tag from a live OCI source
// (E2E_OCI_SOURCE_*) into a HAR HELM registry and reconciles the tag.
func TestMigrateHelmOCILive(t *testing.T) {
	creds := harness.RequireEnv(t)
	src := harness.RequireOCISource(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.SpecFromOCISource(t, src, "HELM", "HELM", "helmoci"))
}
