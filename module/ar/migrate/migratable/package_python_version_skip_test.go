package migratable

import (
	"archive/tar"
	"archive/zip"
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

// minimalPyPIWheel builds a valid .whl (ZIP) containing a METADATA file in the
// .dist-info directory, so File.Migrate's PYTHON wheel branch can extract
// metadata and reach UploadFile.
func minimalPyPIWheel(name, version string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	content := fmt.Sprintf("Metadata-Version: 2.1\nName: %s\nVersion: %s\n\n", name, version)
	distInfo := fmt.Sprintf("%s-%s.dist-info/METADATA", name, version)
	w, _ := zw.Create(distInfo)
	_, _ = w.Write([]byte(content))
	_ = zw.Close()

	return buf.Bytes()
}

// pypiFixtureFiles mirrors the mock_jfrog python-local fixture's shape: a
// per-package index plus real per-version artifact paths under
// requests/<version>/requests-<version>.tar.gz.
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
func newPythonPackageJob(src, dest adp.Adapter, node, unfilteredNode *types.TreeNode, stats *types.TransferStats) *Package {
	return &Package{
		srcRegistry:    "src-reg",
		destRegistry:   "dst-reg",
		srcAdapter:     src,
		destAdapter:    dest,
		artifactType:   types.PYTHON,
		logger:         zerolog.Nop(),
		pkg:            types.Package{Name: "requests", Path: "/"},
		node:           node,
		unfilteredNode: unfilteredNode,
		stats:          stats,
		config:         &types.Config{Concurrency: 2},
	}
}

// TestPackageMigratePythonMixedWindowSkipsOnlyOutOfWindowVersions covers a
// mixed date-filter window: GetVersions returns the PyPI package's full
// unfiltered release history (oldest first), but the tree handed to the
// Package job is the DATE-FILTERED tree — only 2.28.0 and 2.29.0 survived;
// 0.1.1 was pruned. A single out-of-window version must be skipped, not abort
// the whole package: in-window versions must still produce VersionJobs (and
// successful uploads), and Migrate must return nil.
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
	job := newPythonPackageJob(src, dest, root, root, stats)

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
	job := newPythonPackageJob(src, dest, root, root, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err (all-out-of-window package must contribute zero jobs, not fail): %v", err)
	}
	if uploaded := dest.uploadedSnapshot(); len(uploaded) != 0 {
		t.Errorf("expected zero uploads, got: %+v", uploaded)
	}
}

// TestPackageMigratePythonMixedWindowMigratesWholeVersion covers a single PyPI
// release spanning several distribution files (sdist + wheel), surfaced by
// GetVersions as separate version entries sharing Name but with distinct
// Paths. The date filter runs per file, so it can keep the wheel of 2.28.0
// while pruning its sdist. If ANY file of a version is in-scope (survived the
// filter), the WHOLE version is migrated, recovering the pruned sdist from the
// unfiltered tree. This ensures partial versions are never published and
// never discarded.
func TestPackageMigratePythonMixedWindowMigratesWholeVersion(t *testing.T) {
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
	filteredRoot := tree.TransformToTree(filtered)

	// Unfiltered tree: full history including the pruned sdist.
	unfiltered := []types.File{
		{Name: "simple.html", Uri: "/.pypi/simple.html", Size: 64},
		{Name: "requests.html", Uri: "/.pypi/requests/requests.html", Size: 128},
		{Name: "requests-2.28.0.tar.gz", Uri: "/requests/2.28.0/requests-2.28.0.tar.gz", Size: 4000},
		{Name: "requests-2.28.0-py3-none-any.whl", Uri: "/requests/2.28.0/requests-2.28.0-py3-none-any.whl", Size: 4096},
		{Name: "requests-2.29.0.tar.gz", Uri: "/requests/2.29.0/requests-2.29.0.tar.gz", Size: 4200},
	}
	unfilteredRoot := tree.TransformToTree(unfiltered)

	src := &pypiFakeSrc{
		versions: versions,
		content: map[string][]byte{
			"/requests/2.28.0/requests-2.28.0.tar.gz":           minimalPyPIPackageTarGz("requests", "2.28.0"),
			"/requests/2.28.0/requests-2.28.0-py3-none-any.whl": minimalPyPIWheel("requests", "2.28.0"),
			"/requests/2.29.0/requests-2.29.0.tar.gz":           minimalPyPIPackageTarGz("requests", "2.29.0"),
		},
	}
	dest := &pypiFakeDest{}
	stats := &types.TransferStats{}
	job := newPythonPackageJob(src, dest, filteredRoot, unfilteredRoot, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err: %v", err)
	}

	uploaded := dest.uploadedSnapshot()
	// Version 2.28.0 is in-scope (wheel survived), so the WHOLE version
	// migrates (both sdist + wheel). Plus 2.29.0 (fully in-window) = 3 uploads.
	if len(uploaded) != 3 {
		t.Fatalf("expected 3 uploads (both 2.28.0 files + 2.29.0), got %d: %+v", len(uploaded), uploaded)
	}
	has2280sdist := false
	has2280whl := false
	has2290 := false
	for _, uri := range uploaded {
		if uri == "/requests/2.28.0/requests-2.28.0.tar.gz" {
			has2280sdist = true
		}
		if uri == "/requests/2.28.0/requests-2.28.0-py3-none-any.whl" {
			has2280whl = true
		}
		if strings.Contains(uri, "2.29.0") {
			has2290 = true
		}
	}
	if !has2280sdist {
		t.Errorf("expected 2.28.0 sdist (recovered from unfiltered tree) to be uploaded, got: %+v", uploaded)
	}
	if !has2280whl {
		t.Errorf("expected 2.28.0 wheel to be uploaded, got: %+v", uploaded)
	}
	if !has2290 {
		t.Errorf("expected 2.29.0 to be uploaded, got: %+v", uploaded)
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
	job := newPythonPackageJob(src, dest, root, root, stats)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned err: %v", err)
	}
	if uploaded := dest.uploadedSnapshot(); len(uploaded) != 3 {
		t.Fatalf("expected 3 uploads (all versions in-window), got %d: %+v", len(uploaded), uploaded)
	}
}
