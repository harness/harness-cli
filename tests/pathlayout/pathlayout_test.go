//go:build e2e

// Package pathlayout contains end-to-end tests that assert artifacts land at the
// exact destination path/coordinate the migration is expected to produce. These
// guard against path-construction regressions (e.g. %2F encoding of nested
// paths, missing /pkg/ prefixes, or dropped path segments).
package pathlayout

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestGenericNestedPathNoEncoding verifies GENERIC uploads preserve nested path
// segments verbatim at default/default/<uri> (no %2F encoding regression). Both
// a data/ and a bin/ nested file must resolve by their literal slash paths.
func TestGenericNestedPathNoEncoding(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "GENERIC",
		PackageType:    "GENERIC",
		SourceRegistry: "generic-local",
		DestRegistry:   harness.UniqueRegistry(t, "plgeneric"),
		ExpectedRawURIs: []string{
			"default/default/data/config.json",
			"default/default/bin/run.sh",
		},
	})
}

// TestRawDeepPathSucceeds verifies a RAW file with two or more path segments
// uploads and reconciles at its exact source-relative URI.
func TestRawDeepPathSucceeds(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC",
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "plrawdeep"),
		IncludePatterns: []string{"configs/**"},
		ExpectedRawURIs: []string{"configs/v1/config.yaml"},
	})
}

// TestMavenGavFileMembership verifies the Maven GAV coordinate is present and
// that the jar shows up in the destination's per-version file listing (the
// stricter path-layout assertion beyond version existence).
func TestMavenGavFileMembership(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "MAVEN",
		PackageType:    "MAVEN",
		SourceRegistry: "maven-local",
		DestRegistry:   harness.UniqueRegistry(t, "plmaven"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "com.example:sample", Version: "1.0", FileName: "sample-1.0.jar"},
		},
	})
}
