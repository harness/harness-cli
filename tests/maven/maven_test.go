//go:build e2e

// Package maven contains the end-to-end migration test for Maven artifacts:
// MOCK_JFROG maven-local -> HAR MAVEN registry.
package maven

import (
	"testing"

	"github.com/harness/harness-cli/tests/harness"
)

// TestMigrateMaven migrates the maven-local artifacts (jar + pom under the
// standard groupId/artifactId/version layout) and verifies the resulting
// coordinate is present in the destination. Maven derives the destination
// package name from the file path (groupId:artifactId), so the expectation is
// com.example:sample @ 1.0.
func TestMigrateMaven(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.Run(t, bin, creds, harness.Spec{
		ArtifactType:   "MAVEN",
		PackageType:    "MAVEN",
		SourceRegistry: "maven-local",
		DestRegistry:   harness.UniqueRegistry(t, "maven"),
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "com.example:sample", Version: "1.0", FileName: "sample-1.0.jar"},
		},
	})
}
