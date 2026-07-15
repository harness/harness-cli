package migratable

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/rs/zerolog"
)

// anonymousKeychain always resolves to authn.Anonymous. The fake registry.New()
// server requires no auth, but lib.CreateCraneKeychain composes whatever
// GetKeyChain returns into authn.NewMultiKeychain, which calls Resolve on each
// entry unconditionally — a nil Keychain interface (noopAdapter's default)
// panics there, so the OCI tests need a real (if trivial) keychain.
type anonymousKeychain struct{}

func (anonymousKeychain) Resolve(authn.Resource) (authn.Authenticator, error) {
	return authn.Anonymous, nil
}

// ociAdapter is a minimal adapter.Adapter for the OCI/crane path: it only
// needs to hand back a keychain (anonymous, since the fake registry.New()
// server requires no auth) and echo the image path unchanged, since the fake
// registry's host+repo IS the "image path" migrateOCI/copyTagsIndividually
// operate on.
type ociAdapter struct {
	noopAdapter
}

func (ociAdapter) GetKeyChain(string) (authn.Keychain, error) { return anonymousKeychain{}, nil }

func (ociAdapter) GetOCIImagePath(registryHost string, _ string, image string) (string, error) {
	return registryHost + "/" + image, nil
}

// newOCIJob builds a Package job wired directly to a fake registry's
// repositories, bypassing NewPackageJob so the test can set the unexported
// fields it needs (srcRegistry/destRegistry carry the fake server's host).
func newOCIJob(srcHost, dstHost string, overwrite bool) *Package {
	return &Package{
		srcRegistry:  srcHost,
		destRegistry: dstHost,
		srcAdapter:   ociAdapter{},
		destAdapter:  ociAdapter{},
		artifactType: types.DOCKER,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: "repo"},
		stats:        &types.TransferStats{},
		config:       &types.Config{Overwrite: overwrite, Concurrency: 2},
	}
}

// pushRandomImage pushes a small random image to ref (host/repo:tag) and
// returns its digest string.
func pushRandomImage(t *testing.T, ref string) string {
	t.Helper()
	img, err := random.Image(128, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	if err := crane.Push(img, ref); err != nil {
		t.Fatalf("crane.Push(%s): %v", ref, err)
	}
	d, err := img.Digest()
	if err != nil {
		t.Fatalf("img.Digest: %v", err)
	}
	return d.String()
}

func imageDigest(t *testing.T, ref string) v1.Hash {
	t.Helper()
	desc, err := crane.Head(ref)
	if err != nil {
		t.Fatalf("crane.Head(%s): %v", ref, err)
	}
	return desc.Digest
}

// TestCopyTagsIndividually_MovedTagIsCorrected is the core regression test for
// the bug being fixed: a tag ("latest") already exists at the destination
// pointing at an old digest, and the source has since moved that same tag to
// a new image. copyTagsIndividually must detect the digest mismatch and
// re-push, correcting the destination tag — the previous no-clobber based
// logic would have skipped it forever because the tag NAME already existed.
func TestCopyTagsIndividually_MovedTagIsCorrected(t *testing.T) {
	src := httptest.NewServer(registry.New())
	defer src.Close()
	dst := httptest.NewServer(registry.New())
	defer dst.Close()

	srcHost := mustHost(t, src.URL)
	dstHost := mustHost(t, dst.URL)

	srcImage := srcHost + "/repo"
	dstImage := dstHost + "/repo"

	// Destination already has "latest" pointing at the OLD image.
	oldDigest := pushRandomImage(t, dstImage+":latest")

	// Source has since moved "latest" to point at a NEW image.
	newDigest := pushRandomImage(t, srcImage+":latest")
	if newDigest == oldDigest {
		t.Fatalf("test setup bug: expected distinct digests, got the same %s", newDigest)
	}

	job := newOCIJob(srcHost, dstHost, false)
	craneOpts := []crane.Option{}
	res, err := job.copyTagsIndividually(context.Background(), zerolog.Nop(), srcImage, dstImage, craneOpts)
	if err != nil {
		t.Fatalf("copyTagsIndividually returned err: %v", err)
	}
	if res.migrated != 1 {
		t.Errorf("res.migrated = %d, want 1 (moved tag should be re-pushed)", res.migrated)
	}
	if res.skipped != 0 {
		t.Errorf("res.skipped = %d, want 0", res.skipped)
	}

	gotDigest := imageDigest(t, dstImage+":latest")
	if gotDigest.String() != newDigest {
		t.Errorf("destination tag digest = %s, want %s (the moved/new source digest)", gotDigest.String(), newDigest)
	}
}

// TestCopyTagsIndividually_UnchangedTagIsSkipped verifies that a tag whose
// destination digest already matches the source is skipped (counted, but not
// re-pushed).
func TestCopyTagsIndividually_UnchangedTagIsSkipped(t *testing.T) {
	src := httptest.NewServer(registry.New())
	defer src.Close()
	dst := httptest.NewServer(registry.New())
	defer dst.Close()

	srcHost := mustHost(t, src.URL)
	dstHost := mustHost(t, dst.URL)

	srcImage := srcHost + "/repo"
	dstImage := dstHost + "/repo"

	img, err := random.Image(128, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	if err := crane.Push(img, srcImage+":v1"); err != nil {
		t.Fatalf("push src: %v", err)
	}
	if err := crane.Push(img, dstImage+":v1"); err != nil {
		t.Fatalf("push dst: %v", err)
	}
	wantDigest := imageDigest(t, dstImage+":v1")

	job := newOCIJob(srcHost, dstHost, false)
	res, err := job.copyTagsIndividually(context.Background(), zerolog.Nop(), srcImage, dstImage, nil)
	if err != nil {
		t.Fatalf("copyTagsIndividually returned err: %v", err)
	}
	if res.skipped != 1 {
		t.Errorf("res.skipped = %d, want 1", res.skipped)
	}
	if res.migrated != 0 {
		t.Errorf("res.migrated = %d, want 0 (already in sync tag must not be counted as migrated)", res.migrated)
	}

	// Destination content must be unchanged.
	gotDigest := imageDigest(t, dstImage+":v1")
	if gotDigest.String() != wantDigest.String() {
		t.Errorf("destination digest changed: got %s, want unchanged %s", gotDigest.String(), wantDigest.String())
	}
}

// TestCopyTagsIndividually_MissingDestTagIsPushed verifies a tag present only
// at the source is pushed and counted as migrated.
func TestCopyTagsIndividually_MissingDestTagIsPushed(t *testing.T) {
	src := httptest.NewServer(registry.New())
	defer src.Close()
	dst := httptest.NewServer(registry.New())
	defer dst.Close()

	srcHost := mustHost(t, src.URL)
	dstHost := mustHost(t, dst.URL)

	srcImage := srcHost + "/repo"
	dstImage := dstHost + "/repo"

	srcDigest := pushRandomImage(t, srcImage+":v1")

	job := newOCIJob(srcHost, dstHost, false)
	res, err := job.copyTagsIndividually(context.Background(), zerolog.Nop(), srcImage, dstImage, nil)
	if err != nil {
		t.Fatalf("copyTagsIndividually returned err: %v", err)
	}
	if res.migrated != 1 {
		t.Errorf("res.migrated = %d, want 1", res.migrated)
	}
	if res.skipped != 0 {
		t.Errorf("res.skipped = %d, want 0", res.skipped)
	}

	gotDigest := imageDigest(t, dstImage+":v1")
	if gotDigest.String() != srcDigest {
		t.Errorf("destination tag digest = %s, want %s", gotDigest.String(), srcDigest)
	}
}

// phantomTagServer wraps a registry.New() handler and injects an extra
// "phantom" tag name into the tags-list response for repo, without ever
// registering a manifest for it. This models a tag whose source manifest was
// garbage-collected: ListTags enumerates it, but any manifest GET/HEAD for it
// naturally 404s (MANIFEST_UNKNOWN) against the underlying fake registry,
// since it was never pushed.
func phantomTagServer(repo, phantomTag string) *httptest.Server {
	inner := registry.New()
	tagsPath := "/v2/" + repo + "/tags/list"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == tagsPath {
			rec := httptest.NewRecorder()
			inner.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				maps.Copy(w.Header(), rec.Header())
				w.WriteHeader(rec.Code)
				_, _ = w.Write(rec.Body.Bytes())
				return
			}
			var parsed struct {
				Name string   `json:"name"`
				Tags []string `json:"tags"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			parsed.Tags = append(parsed.Tags, phantomTag)
			sort.Strings(parsed.Tags)
			body, _ := json.Marshal(parsed)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			return
		}
		inner.ServeHTTP(w, r)
	}))
}

// TestCopyTagsIndividually_OrphanedSourceTagIsSkippedNotFailed simulates a
// source tag whose manifest has been garbage-collected (registry answers
// MANIFEST_UNKNOWN on both HEAD and GET, since crane.Digest falls back to GET
// on a HEAD failure). copyTagsIndividually must count it as skipped and
// return a nil error, so one bad tag does not fail the whole image, while a
// healthy sibling tag still migrates.
func TestCopyTagsIndividually_OrphanedSourceTagIsSkippedNotFailed(t *testing.T) {
	src := phantomTagServer("repo", "orphan")
	defer src.Close()
	dst := httptest.NewServer(registry.New())
	defer dst.Close()

	srcHost := mustHost(t, src.URL)
	dstHost := mustHost(t, dst.URL)

	srcImage := srcHost + "/repo"
	dstImage := dstHost + "/repo"

	// Healthy tag that should still migrate. The "orphan" tag is injected by
	// phantomTagServer into ListTags only — its manifest was never pushed, so
	// resolving its digest 404s with MANIFEST_UNKNOWN.
	healthyDigest := pushRandomImage(t, srcImage+":v1")

	job := newOCIJob(srcHost, dstHost, false)
	res, err := job.copyTagsIndividually(context.Background(), zerolog.Nop(), srcImage, dstImage, nil)
	if err != nil {
		t.Fatalf("copyTagsIndividually returned err (orphaned tag must not fail the image): %v", err)
	}
	if res.skipped != 1 {
		t.Errorf("res.skipped = %d, want 1 (the orphaned tag)", res.skipped)
	}
	if res.migrated != 1 {
		t.Errorf("res.migrated = %d, want 1 (the healthy sibling tag)", res.migrated)
	}
	if res.failed != 0 {
		t.Errorf("res.failed = %d, want 0", res.failed)
	}

	gotDigest := imageDigest(t, dstImage+":v1")
	if gotDigest.String() != healthyDigest {
		t.Errorf("healthy tag digest = %s, want %s", gotDigest.String(), healthyDigest)
	}
}

// TestMigrateOCI_OverwriteFalseCorrectsMovedTag is a behavioral check on
// migrateOCI itself (not just copyTagsIndividually): with Overwrite=false, a
// moved tag must be corrected end-to-end through Migrate's OCI branch, proving
// the bulk crane.CopyRepository fast path (which cannot detect a moved tag) is
// bypassed rather than merely falling back to it after a failure.
func TestMigrateOCI_OverwriteFalseCorrectsMovedTag(t *testing.T) {
	src := httptest.NewServer(registry.New())
	defer src.Close()
	dst := httptest.NewServer(registry.New())
	defer dst.Close()

	srcHost := mustHost(t, src.URL)
	dstHost := mustHost(t, dst.URL)

	srcImage := srcHost + "/repo"
	dstImage := dstHost + "/repo"

	pushRandomImage(t, dstImage+":latest")
	newDigest := pushRandomImage(t, srcImage+":latest")

	job := newOCIJob(srcHost, dstHost, false)
	job.migrateOCI(context.Background(), zerolog.Nop())

	if len(job.stats.FileStats) != 1 {
		t.Fatalf("expected exactly 1 FileStat for the image, got %d: %+v", len(job.stats.FileStats), job.stats.FileStats)
	}
	stat := job.stats.FileStats[0]
	if stat.Status != types.StatusSuccess {
		t.Errorf("stat.Status = %s, want Success (err=%q)", stat.Status, stat.Error)
	}

	gotDigest := imageDigest(t, dstImage+":latest")
	if gotDigest.String() != newDigest {
		t.Errorf("destination tag digest = %s, want %s (moved tag must be corrected)", gotDigest.String(), newDigest)
	}
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return u.Host
}

// Sanity guard: withoutNoClobber must append WithNoClobber(false) LAST so it
// wins over an earlier WithNoClobber(true), verified indirectly through
// observable push behavior against a fake registry that already has the
// destination tag under a different digest — with plain craneOpts (no
// override) the push would be refused; with the no-clobber-stripped push
// options it must succeed.
func TestWithoutNoClobber_AllowsOverwrite(t *testing.T) {
	// crane's no-clobber check is implemented in crane.Copy/CopyRepository
	// (crane.Push has no such check), so exercise it via crane.Copy across two
	// fake registries, mirroring how copyTagsIndividually actually pushes.
	src := httptest.NewServer(registry.New())
	defer src.Close()
	dst := httptest.NewServer(registry.New())
	defer dst.Close()
	srcHost := mustHost(t, src.URL)
	dstHost := mustHost(t, dst.URL)
	srcImage := srcHost + "/repo"
	dstImage := dstHost + "/repo"

	pushRandomImage(t, dstImage+":latest")
	newDigest := pushRandomImage(t, srcImage+":latest")

	blockedOpts := []crane.Option{crane.WithNoClobber(true)}
	if err := crane.Copy(srcImage+":latest", dstImage+":latest", blockedOpts...); err == nil {
		t.Fatalf("expected no-clobber to refuse copying onto an existing tag name, but it succeeded")
	}
	if got := imageDigest(t, dstImage+":latest"); got.String() == newDigest {
		t.Fatalf("test setup bug: no-clobber unexpectedly overwrote the tag, can't demonstrate the fix")
	}

	allowedOpts := withoutNoClobber(blockedOpts)
	if err := crane.Copy(srcImage+":latest", dstImage+":latest", allowedOpts...); err != nil {
		t.Fatalf("withoutNoClobber copy should succeed, got err: %v", err)
	}

	got := imageDigest(t, dstImage+":latest")
	if got.String() != newDigest {
		t.Errorf("digest after overwrite = %s, want %s", got.String(), newDigest)
	}
}
