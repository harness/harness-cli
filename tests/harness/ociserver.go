package harness

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// OCIRegistry is an in-memory OCI registry served over HTTP for tests. It backs
// the offline DOCKER/HELM copy tests, which exercise the exact crane primitives
// the migration (CopyRepository) and reconciliation (ListTags) use — without
// requiring a live HAR OCI endpoint. Because it is served over plain HTTP, all
// crane calls must use crane.Insecure.
type OCIRegistry struct {
	t      *testing.T
	server *httptest.Server
	// Host is the registry authority (host:port), e.g. 127.0.0.1:54321.
	Host string
}

// StartOCIRegistry starts an in-memory OCI registry and registers cleanup.
func StartOCIRegistry(t *testing.T) *OCIRegistry {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	return &OCIRegistry{
		t:      t,
		server: srv,
		Host:   strings.TrimPrefix(srv.URL, "http://"),
	}
}

// Repo returns the host-qualified repository path (no tag).
func (r *OCIRegistry) Repo(repo string) string { return r.Host + "/" + repo }

// Ref returns the host-qualified reference host/repo:tag.
func (r *OCIRegistry) Ref(repo, tag string) string { return r.Repo(repo) + ":" + tag }

// PushRandomImage pushes a random single-architecture image at repo:tag and
// returns it.
func (r *OCIRegistry) PushRandomImage(repo, tag string) v1.Image {
	r.t.Helper()
	img, err := random.Image(1024, 2)
	if err != nil {
		r.t.Fatalf("build random image: %v", err)
	}
	if err := crane.Push(img, r.Ref(repo, tag), crane.Insecure); err != nil {
		r.t.Fatalf("push %s: %v", r.Ref(repo, tag), err)
	}
	return img
}

// PushRandomIndex pushes a random multi-architecture image index (with the given
// number of platform manifests) at repo:tag.
func (r *OCIRegistry) PushRandomIndex(repo, tag string, platforms int) {
	r.t.Helper()
	idx, err := random.Index(1024, 1, int64(platforms))
	if err != nil {
		r.t.Fatalf("build random index: %v", err)
	}
	ref, err := name.ParseReference(r.Ref(repo, tag), name.Insecure)
	if err != nil {
		r.t.Fatalf("parse ref %s: %v", r.Ref(repo, tag), err)
	}
	if err := remote.WriteIndex(ref, idx); err != nil {
		r.t.Fatalf("write index %s: %v", r.Ref(repo, tag), err)
	}
}

// Tags lists the tags for a repository (the reconcile primitive).
func (r *OCIRegistry) Tags(repo string) []string {
	r.t.Helper()
	tags, err := crane.ListTags(r.Repo(repo), crane.Insecure)
	if err != nil {
		r.t.Fatalf("list tags for %s: %v", r.Repo(repo), err)
	}
	return tags
}

// IndexManifestCount returns the number of child manifests of the index at
// repo:tag (used to assert multi-arch copies preserved every platform).
func (r *OCIRegistry) IndexManifestCount(repo, tag string) int {
	r.t.Helper()
	ref, err := name.ParseReference(r.Ref(repo, tag), name.Insecure)
	if err != nil {
		r.t.Fatalf("parse ref %s: %v", r.Ref(repo, tag), err)
	}
	idx, err := remote.Index(ref)
	if err != nil {
		r.t.Fatalf("read index %s: %v", r.Ref(repo, tag), err)
	}
	im, err := idx.IndexManifest()
	if err != nil {
		r.t.Fatalf("index manifest %s: %v", r.Ref(repo, tag), err)
	}
	return len(im.Manifests)
}
