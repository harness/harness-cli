//go:build e2e

// Package composer contains the end-to-end migration test for Composer
// artifacts: MOCK_JFROG composer-local -> HAR COMPOSER registry.
package composer

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateComposer migrates the vendor/package Composer archive and verifies
// the version is present in the destination. The destination name/version are
// read by the migration from the composer.json inside the zip.
func TestMigrateComposer(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "COMPOSER",
		PackageType:    "COMPOSER",
		SourceRegistry: "composer-local",
		DestRegistry:   harness.UniqueRegistry(t, "composer"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "vendor/package", Version: "1.0.0"},
		},
	})
}
