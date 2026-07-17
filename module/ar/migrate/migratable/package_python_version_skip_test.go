package migratable

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// pypiFakeSrc serves the PyPI package's full (unfiltered) release history via
// GetVersions and per-version tarball bytes via DownloadFile, mirroring the
// real jfrog adapter: GetVersions reads .pypi/<pkg>/<pkg>.html directly and
// returns every listed release regardless of what the date filter kept in the
// tree.
type pypiFakeSrc struct {
	noopAdapter
	versions []types.Version
	content  map[string][]byte // version.Path -> tarball bytes
}

func (s *pypiFakeSrc) GetVersions(
	types.Package, *types.TreeNode, string, string, types.ArtifactType,
) ([]types.Version, error) {
	return s.versions, nil
}

func (s *pypiFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

// pypiFakeDest records every file uploaded, keyed by the source file's Uri.
// Package.Migrate runs Version jobs concurrently (config.Concurrency), so
// UploadFile can be called from multiple goroutines at once.
type pypiFakeDest struct {
	noopAdapter
	mu       sync.Mutex
	uploaded []string
}

func (d *pypiFakeDest) UploadFile(
	_ string,
	file io.ReadCloser,
	f *types.File,
	_ http.Header,
	_ string,
	_ string,
	_ types.ArtifactType,
	_ map[string]interface{},
) error {
	if file != nil {
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
	}
	d.mu.Lock()
	d.uploaded = append(d.uploaded, f.Uri)
	d.mu.Unlock()
	return nil
}

func (d *pypiFakeDest) uploadedSnapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.uploaded...)
}

// minimalPyPIPackageTarGz builds a valid .tar.gz containing a PKG-INFO file,
// so File.Migrate's PYTHON branch can extract metadata and reach UploadFile.
func minimalPyPIPackageTarGz(name, version string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	content := fmt.Sprintf("Metadata-Version: 2.1\nName: %s\nVersion: %s\n\n", name, version)
	hdr := &tar.Header{
		Name: fmt.Sprintf("%s-%s/PKG-INFO", name, version),
		Mode: 0644,
		Size: int64(len(content)),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(content))
	_ = tw.Close()
	_ = gz.Close()

	return buf.Bytes()
}

// pypiFixtureFiles mirrors the mock_jfrog python-local fixture's shape (see
// AH-4518 Defect 1/2): a per-package index plus real per-version artifact
// paths under requests/<version>/requests-<version>.tar.gz.
func pypiFixtureFiles() []types.File {
	return []types.File{
		{Name: "simple.html", Uri: "/.pypi/simple.html", Size: 64},
		{Name: "requests.html", Uri: "/.pypi/requests/requests.html", Size: 128},
		{Name: "requests-0.1.1.tar.gz", Uri: "/requests/0.1.1/requests-0.1.1.tar.gz", Size: 100},
		{Name: "requests-2.28.0.tar.gz", Uri: "/requests/2.28.0/requests-2.28.0.tar.gz", Size: 4096},
		{Name: "requests-2.29.0.tar.gz", Uri: "/requests/2.29.0/requests-2.29.0.tar.gz", Size: 4200},
	}
}

// pypiFixtureVersions is what the (unfiltered) jfrog PYTHON GetVersions
// returns for the "requests" package: every release in
// .pypi/requests/requests.html, oldest first, regardless of the date filter.
func pypiFixtureVersions() []types.Version {
	return []types.Version{
		{Pkg: "requests", Name: "0.1.1", Path: "requests/0.1.1/requests-0.1.1.tar.gz"},
		{Pkg: "requests", Name: "2.28.0", Path: "requests/2.28.0/requests-2.28.0.tar.gz"},
		{Pkg: "requests", Name: "2.29.0", Path: "requests/2.29.0/requests-2.29.0.tar.gz"},
	}
}

// newPythonPackageJob builds a Package job directly (bypassing NewPackageJob)
// with artifactType PYTHON, wired to the given adapters and node.
func newPythonPackageJob(src, dest adp.Adapter, node *types.TreeNode, stats *types.TransferStats) *Package {
	return &Package{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.PYTHON,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: "requests", Path: "/"},
		node:         node,
		stats:        stats,
		config:       &types.Config{Concurrency: 2},
	}
}

// TestPackageMigratePythonMixedWindowSkipsOnlyOutOfWindowVersions is the core
// AH-4518 Defect 2 regression test: GetVersions returns the PyPI package's
// full unfiltered release history (oldest first, per the report's spf-pylib
// trace), but the tree handed to the Package job is the DATE-FILTERED tree —
// only 2.28.0 and 2.29.0 survived; 0.1.1 was pruned. A single out-of-window
// version must be skipped, not abort the whole package: in-window versions
// must still produce VersionJobs (and successful uploads), and Migrate must
// return nil.
func TestPackageMigratePythonMixedWindowSkipsOnlyOutOfWindowVersions(t *testing.T) {
	allFiles := pypiFixtureFiles()
	// Simulate the date filter: drop the out-of-window artifact
	// (requests-0.1.1.tar.gz) from the tree, but keep the index files (Defect 1
	// exemption) and the in-window artifacts.
	var filtered []types.File
	for _, f := range allFiles {
		if f.Uri == "/requests/0.1.1/requests-0.1.1.tar.gz" {
			continue
		}
		filtered = append(filtered, f)
	}
	// For PyPI, pkg.Path is "/" (adapter.go GetPackages PYTHON branch), so
	// registry.go's per-package job node is the tree ROOT, not a "requests"
	// subtree - version.Path values are rooted at the registry root too.
	root := tree.TransformToTree(filtered)

	src := &pypiFakeSrc{
		versions: pypiFixtureVersions(),
		content: map[string][]byte{
			"/requests/2.28.0/requests-2.28.0.tar.gz": minimalPyPIPackageTarGz("requests", "2.28.0"),
			"/requests/2.29.0/requests-2.29.0.tar.gz": minimalPyPIPackageTarGz("requests", "2.29.0"),
		},
	}
	dest := &pypiFakeDest{}
	stats := &types.TransferStats{}
	job := newPythonPackageJob(src, dest, root, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err (a single out-of-window version must not abort the package): %v", err)
	}

	// Only the two in-window versions should have produced uploads.
	uploaded := dest.uploadedSnapshot()
	if len(uploaded) != 2 {
		t.Fatalf("expected 2 uploads (in-window versions only), got %d: %+v", len(uploaded), uploaded)
	}
	for _, uri := range uploaded {
		if strings.Contains(uri, "0.1.1") {
			t.Errorf("out-of-window version 0.1.1 must not be uploaded, got uploads: %+v", uploaded)
		}
	}
}

// TestPackageMigratePythonAllOutOfWindowContributesZeroJobs: every listed
// version is absent from the filtered tree (the whole package's history is
// out of window) → Migrate returns nil and nothing is uploaded, but this must
// NOT be treated as a failure.
func TestPackageMigratePythonAllOutOfWindowContributesZeroJobs(t *testing.T) {
	// Filtered tree contains only the index files - no artifact survived the
	// date filter for this package, so "requests" has no node at all.
	filtered := []types.File{
		{Uri: "/.pypi/simple.html", Size: 64},
		{Uri: "/.pypi/requests/requests.html", Size: 128},
	}
	root := tree.TransformToTree(filtered)

	src := &pypiFakeSrc{versions: pypiFixtureVersions(), content: map[string][]byte{}}
	dest := &pypiFakeDest{}
	stats := &types.TransferStats{}
	job := newPythonPackageJob(src, dest, root, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err (all-out-of-window package must contribute zero jobs, not fail): %v", err)
	}
	if uploaded := dest.uploadedSnapshot(); len(uploaded) != 0 {
		t.Errorf("expected zero uploads, got: %+v", uploaded)
	}
}

// TestPackageMigratePythonPartialVersionDiscarded is the multi-file-version
// (all-or-nothing) regression: a single PyPI release spans several distribution
// files (sdist + wheel), surfaced by GetVersions as separate version entries
// sharing Name but with distinct Paths. The date filter runs per file, so it
// can keep the wheel of 2.28.0 while pruning its sdist. Migrating only the
// survivor would publish a PARTIAL version, so the whole 2.28.0 version must be
// discarded — while a fully-in-window version (2.29.0) still migrates in full.
func TestPackageMigratePythonPartialVersionDiscarded(t *testing.T) {
	// Full source history: 2.28.0 has two distributions (sdist + wheel), 2.29.0
	// has one. GetVersions returns one entry per distribution file.
	versions := []types.Version{
		{Pkg: "requests", Name: "2.28.0", Path: "/requests/2.28.0/requests-2.28.0.tar.gz"},
		{Pkg: "requests", Name: "2.28.0", Path: "/requests/2.28.0/requests-2.28.0-py3-none-any.whl"},
		{Pkg: "requests", Name: "2.29.0", Path: "/requests/2.29.0/requests-2.29.0.tar.gz"},
	}

	// Filtered tree: 2.28.0's sdist was pruned by the date filter; its wheel and
	// all of 2.29.0 survived (plus the exempt index files).
	filtered := []types.File{
		{Name: "simple.html", Uri: "/.pypi/simple.html", Size: 64},
		{Name: "requests.html", Uri: "/.pypi/requests/requests.html", Size: 128},
		{Name: "requests-2.28.0-py3-none-any.whl", Uri: "/requests/2.28.0/requests-2.28.0-py3-none-any.whl", Size: 4096},
		{Name: "requests-2.29.0.tar.gz", Uri: "/requests/2.29.0/requests-2.29.0.tar.gz", Size: 4200},
	}
	root := tree.TransformToTree(filtered)

	src := &pypiFakeSrc{
		versions: versions,
		content: map[string][]byte{
			"/requests/2.28.0/requests-2.28.0-py3-none-any.whl": minimalPyPIPackageTarGz("requests", "2.28.0"),
			"/requests/2.29.0/requests-2.29.0.tar.gz":           minimalPyPIPackageTarGz("requests", "2.29.0"),
		},
	}
	dest := &pypiFakeDest{}
	stats := &types.TransferStats{}
	job := newPythonPackageJob(src, dest, root, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err (a partial version must be discarded, not fail the package): %v", err)
	}

	uploaded := dest.uploadedSnapshot()
	// Only 2.29.0 (fully in window) should migrate; both 2.28.0 files must be
	// absent — the surviving wheel must NOT be uploaded on its own.
	if len(uploaded) != 1 {
		t.Fatalf("expected exactly 1 upload (only the fully-in-window version), got %d: %+v", len(uploaded), uploaded)
	}
	for _, uri := range uploaded {
		if strings.Contains(uri, "2.28.0") {
			t.Errorf("partial version 2.28.0 must be discarded entirely, but a 2.28.0 file was uploaded: %+v", uploaded)
		}
	}
	if !strings.Contains(uploaded[0], "2.29.0") {
		t.Errorf("expected the fully-in-window 2.29.0 to migrate, got: %+v", uploaded)
	}
}

// TestPackageMigratePythonAllInWindowUnchanged guards against over-skipping:
// when every listed version resolves in the tree (no date filter pruning),
// all versions must still migrate exactly as before this fix.
func TestPackageMigratePythonAllInWindowUnchanged(t *testing.T) {
	allFiles := pypiFixtureFiles()
	root := tree.TransformToTree(allFiles)

	src := &pypiFakeSrc{
		versions: pypiFixtureVersions(),
		content: map[string][]byte{
			"/requests/0.1.1/requests-0.1.1.tar.gz":   minimalPyPIPackageTarGz("requests", "0.1.1"),
			"/requests/2.28.0/requests-2.28.0.tar.gz": minimalPyPIPackageTarGz("requests", "2.28.0"),
			"/requests/2.29.0/requests-2.29.0.tar.gz": minimalPyPIPackageTarGz("requests", "2.29.0"),
		},
	}
	dest := &pypiFakeDest{}
	stats := &types.TransferStats{}
	job := newPythonPackageJob(src, dest, root, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err: %v", err)
	}
	if uploaded := dest.uploadedSnapshot(); len(uploaded) != 3 {
		t.Fatalf("expected 3 uploads (all versions in-window), got %d: %+v", len(uploaded), uploaded)
	}
}
