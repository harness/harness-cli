// Package ocismoke contains OFFLINE, hermetic tests for the OCI migration
// mechanics. Unlike the rest of tests/ (which are e2e-tagged and require live
// HAR credentials), these run in the normal `go test` lane against two
// in-memory OCI registries. They exercise the exact crane primitives the
// migration uses for DOCKER/HELM — crane.CopyRepository (bulk copy) and
// crane.ListTags (reconcile) — plus no-clobber (overwrite=false) and multi-arch
// behavior, without needing a real registry.
//
// Note: a live HTTP mock source cannot be combined with a live HTTPS HAR
// destination through the production copy path (crane.Insecure forces HTTP on
// both refs), so these tests copy between two mock registries to validate the
// copy/list logic that the migration and reconcile rely on.
package ocismoke

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/harness/harness-cli/tests/harness"
)

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestOCICopySingleTag copies a single-tag repository and verifies the tag lands
// in the destination (crane.CopyRepository + crane.ListTags).
func TestOCICopySingleTag(t *testing.T) {
	src := harness.StartOCIRegistry(t)
	dst := harness.StartOCIRegistry(t)

	src.PushRandomImage("app", "v1")

	if err := crane.CopyRepository(src.Repo("app"), dst.Repo("app"), crane.Insecure); err != nil {
		t.Fatalf("copy repository: %v", err)
	}

	tags := dst.Tags("app")
	if !contains(tags, "v1") {
		t.Errorf("expected tag v1 in destination, got %v", tags)
	}
}

// TestOCICopyMultipleTags copies a repository with several tags and verifies all
// of them are present in the destination.
func TestOCICopyMultipleTags(t *testing.T) {
	src := harness.StartOCIRegistry(t)
	dst := harness.StartOCIRegistry(t)

	want := []string{"v1", "v2", "v3"}
	for _, tag := range want {
		src.PushRandomImage("app", tag)
	}

	if err := crane.CopyRepository(src.Repo("app"), dst.Repo("app"), crane.Insecure); err != nil {
		t.Fatalf("copy repository: %v", err)
	}

	got := dst.Tags("app")
	for _, tag := range want {
		if !contains(got, tag) {
			t.Errorf("expected tag %s in destination, got %v", tag, got)
		}
	}
}

// TestOCICopyMultiArchIndex copies a multi-architecture image index and verifies
// every platform manifest is preserved in the destination.
func TestOCICopyMultiArchIndex(t *testing.T) {
	src := harness.StartOCIRegistry(t)
	dst := harness.StartOCIRegistry(t)

	const platforms = 3
	src.PushRandomIndex("app", "multi", platforms)

	if err := crane.CopyRepository(src.Repo("app"), dst.Repo("app"), crane.Insecure); err != nil {
		t.Fatalf("copy repository: %v", err)
	}

	if !contains(dst.Tags("app"), "multi") {
		t.Fatalf("expected tag multi in destination, got %v", dst.Tags("app"))
	}
	if n := dst.IndexManifestCount("app", "multi"); n != platforms {
		t.Errorf("expected %d platform manifests, got %d", platforms, n)
	}
}

// TestOCINoClobberSkipsExisting mirrors overwrite=false: an already-present tag
// in the destination must NOT be overwritten by the copy.
func TestOCINoClobberSkipsExisting(t *testing.T) {
	src := harness.StartOCIRegistry(t)
	dst := harness.StartOCIRegistry(t)

	// Distinct images at the same repo:tag on source vs destination.
	original := dst.PushRandomImage("app", "v1")
	src.PushRandomImage("app", "v1")
	src.PushRandomImage("app", "v2")

	originalDigest, err := original.Digest()
	if err != nil {
		t.Fatalf("digest of original: %v", err)
	}

	if err := crane.CopyRepository(src.Repo("app"), dst.Repo("app"),
		crane.Insecure, crane.WithNoClobber(true)); err != nil {
		t.Fatalf("copy repository (no-clobber): %v", err)
	}

	// v2 is new and must be copied.
	if !contains(dst.Tags("app"), "v2") {
		t.Errorf("expected new tag v2 to be copied, got %v", dst.Tags("app"))
	}

	// v1 pre-existed and must be left untouched (not clobbered).
	gotDigest, err := crane.Digest(dst.Ref("app", "v1"), crane.Insecure)
	if err != nil {
		t.Fatalf("digest of destination v1: %v", err)
	}
	if gotDigest != originalDigest.String() {
		t.Errorf("no-clobber: destination v1 was overwritten (got %s, want %s)", gotDigest, originalDigest.String())
	}
}
