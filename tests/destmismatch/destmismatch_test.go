//go:build e2e

// Package destmismatch contains end-to-end tests where the destination registry
// type does not match the source artifact type. These assert the migration does
// NOT produce a usable artifact of the source type (the CLI may still exit 0, so
// the negative reconcile is what proves the mismatch was not silently accepted),
// and that provisioning an unsupported package type fails outright.
package destmismatch

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMavenToGeneric migrates Maven artifacts into a GENERIC registry. The Maven
// coordinate must not resolve as a MAVEN version in the destination.
func TestMavenToGeneric(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:   "MAVEN",
		PackageType:    "GENERIC",
		SourceRegistry: "maven-local",
		DestRegistry:   harness.UniqueRegistry(t, "mmmaven2gen"),
		NotExpectedFiles: []harness.ExpectedFile{
			{Pkg: "com.example:sample", Version: "1.0"},
		},
	})
}

// TestPythonToNuget migrates Python sdists into a NUGET registry. The Python
// package must not resolve as a PYTHON version in the destination.
func TestPythonToNuget(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:   "PYTHON",
		PackageType:    "NUGET",
		SourceRegistry: "python-local",
		DestRegistry:   harness.UniqueRegistry(t, "mmpy2nuget"),
		NotExpectedFiles: []harness.ExpectedFile{
			{Pkg: "requests", Version: "2.28.0"},
		},
	})
}

// TestUnsupportedPackageTypeCreateFails asserts the management API rejects
// creating a registry with an unsupported package type (create fails fast,
// before any migration is attempted).
func TestUnsupportedPackageTypeCreateFails(t *testing.T) {
	creds := harness.RequireEnv(t)
	harness.ApplyGlobalConfig(creds)
	harness.EnsureProject(t, creds)
	harness.ExpectCreateRegistryFails(t, creds, harness.UniqueRegistry(t, "mmbadtype"), "NOT_A_REAL_TYPE")
}
