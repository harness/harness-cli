//go:build e2e

// Package gomod contains the end-to-end migration test for Go modules:
// MOCK_JFROG go-local -> HAR GO registry.
package gomod

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateGo migrates a single Go module version (zip + mod + info) and
// verifies the module version is present in the destination. The module path and
// version are derived from the source /@v/ layout.
func TestMigrateGo(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "GO",
		PackageType:    "GO",
		SourceRegistry: "go-local",
		DestRegistry:   harness.UniqueRegistry(t, "go"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "github.com/example/mod", Version: "v1.0.0"},
		},
	})
}
