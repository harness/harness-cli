//go:build e2e

// Package puppet contains the end-to-end migration test for Puppet modules:
// MOCK_JFROG puppet-local -> HAR PUPPET registry.
package puppet

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigratePuppet migrates the puppet-local modules and verifies both
// puppetlabs-stdlib versions are present in the destination. The module name
// mirrors the metadata.json "name" field embedded in the fixture tarballs.
func TestMigratePuppet(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "PUPPET",
		PackageType:    "PUPPET",
		SourceRegistry: "puppet-local",
		DestRegistry:   harness.UniqueRegistry(t, "puppet"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "puppetlabs-stdlib", Version: "9.4.1"},
			{Pkg: "puppetlabs-stdlib", Version: "9.5.0"},
		},
	})
}
