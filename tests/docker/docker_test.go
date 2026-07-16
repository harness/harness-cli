//go:build e2e

// Package docker contains live end-to-end DOCKER (OCI) migration tests. They
// migrate from a real OCI source registry (JFrog by default) into HAR and are
// gated on E2E_OCI_SOURCE_* environment variables — when unset, every test
// skips and the offline tests/ocismoke package covers the copy mechanics.
package docker

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateDockerLive migrates a single image tag from a live OCI source
// (E2E_OCI_SOURCE_*) into a HAR DOCKER registry and reconciles the tag.
func TestMigrateDockerLive(t *testing.T) {
	creds := harness.RequireEnv(t)
	src := harness.RequireOCISource(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.SpecFromOCISource(t, src, "DOCKER", "DOCKER", "docker"))
}
