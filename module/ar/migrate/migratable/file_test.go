package migratable

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestMigratePythonFileMetadataScalars mirrors how a customer migrates PYTHON
// artifacts with hc registry migrate:
//
//  1. Source registry is python-local (MOCK_JFROG in e2e; JFrog in production).
//  2. The engine schedules a per-file job for requests-2.28.0.tar.gz — the same
//     sdist exercised by tests/python/python_test.go.
//  3. File.Migrate downloads the sdist, extracts PKG-INFO, builds the metadata
//     map, and uploads to HAR.
//
// This test runs that full file-migration path with a stub destination and
// asserts the metadata map handed to upload uses string scalars for single-value
// PKG-INFO headers (name, version) and []string for repeated Classifier lines.
func TestMigratePythonFileMetadataScalars(t *testing.T) {
	t.Helper()

	const (
		srcRegistry  = "python-local"
		destRegistry = "dest-python-registry"
		fileURI      = "/requests/2.28.0/requests-2.28.0.tar.gz"
	)

	src := &pythonFakeSrc{
		content: map[string][]byte{
			fileURI: pythonSdistTarball(t, "requests", "2.28.0"),
		},
	}
	dest := &pythonMetadataDest{}

	file := &types.File{
		Registry: srcRegistry,
		Name:     "requests-2.28.0.tar.gz",
		Uri:      fileURI,
		Size:     4096,
	}
	stats := &types.TransferStats{}
	cfg := &types.Config{Overwrite: true}

	job := NewFileJob(
		src,
		dest,
		srcRegistry,
		destRegistry,
		types.PYTHON,
		types.Package{Name: "requests"},
		types.Version{Name: "2.28.0"},
		nil,
		file,
		stats,
		nil,
		cfg,
		types.RegistryInfo{},
		nil,
	)

	ctx := context.Background()
	if err := job.Pre(ctx); err != nil {
		t.Fatalf("pre-migration: %v", err)
	}
	if err := job.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if dest.metadata == nil {
		t.Fatal("destination upload was not called; expected File.Migrate to upload PYTHON sdist")
	}

	assertMapString(t, dest.metadata, "name", "requests")
	assertMapString(t, dest.metadata, "version", "2.28.0")
	assertMapStringSlice(t, dest.metadata, "classifiers", []string{
		"Development Status :: 5 - Production/Stable",
		"License :: OSI Approved :: Apache Software License",
	})

	if len(stats.FileStats) != 1 {
		t.Fatalf("expected 1 file stat, got %d", len(stats.FileStats))
	}
	if stats.FileStats[0].Status != types.StatusSuccess {
		t.Fatalf("expected successful upload stat, got %s (%s)", stats.FileStats[0].Status, stats.FileStats[0].Error)
	}
}

// TestGeneratePythonMetadataMapScalarHeaders is a focused regression for the
// metadata-map builder used inside File.Migrate's PYTHON branch.
func TestGeneratePythonMetadataMapScalarHeaders(t *testing.T) {
	t.Helper()

	tmp := t.TempDir()
	pkgPath := filepath.Join(tmp, "requests-2.28.0.tar.gz")
	if err := os.WriteFile(pkgPath, pythonSdistTarball(t, "requests", "2.28.0"), 0o644); err != nil {
		t.Fatalf("write temp package: %v", err)
	}

	metadata := "" +
		"Metadata-Version: 2.1\r\n" +
		"Name: requests\r\n" +
		"Version: 2.28.0\r\n" +
		"Classifier: Development Status :: 5 - Production/Stable\r\n" +
		"Classifier: License :: OSI Approved :: Apache Software License\r\n" +
		"\r\n" +
		"A HTTP library for Python.\r\n"

	got, err := generatePythonMetadataMap(metadata, pkgPath)
	if err != nil {
		t.Fatalf("generatePythonMetadataMap: %v", err)
	}

	assertMapString(t, got, "name", "requests")
	assertMapString(t, got, "version", "2.28.0")
	assertMapStringSlice(t, got, "classifiers", []string{
		"Development Status :: 5 - Production/Stable",
		"License :: OSI Approved :: Apache Software License",
	})
}

// pythonFakeSrc serves PYTHON sdists keyed by source URI, matching how the JFrog
// adapter's DownloadFile resolves /requests/<version>/requests-<version>.tar.gz.
type pythonFakeSrc struct {
	noopAdapter
	content map[string][]byte
}

func (s *pythonFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/gzip")
	return io.NopCloser(bytes.NewReader(b)), header, nil
}

// pythonMetadataDest records the metadata map a customer upload would send to HAR.
type pythonMetadataDest struct {
	noopAdapter
	metadata map[string]interface{}
}

func (d *pythonMetadataDest) UploadFile(
	_ string,
	file io.ReadCloser,
	_ *types.File,
	_ http.Header,
	_ string,
	_ string,
	artifactType types.ArtifactType,
	metadata map[string]interface{},
) error {
	if artifactType != types.PYTHON {
		return fmt.Errorf("unexpected artifact type %s", artifactType)
	}
	if file != nil {
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
	}
	d.metadata = metadata
	return nil
}

// pythonSdistTarball builds the same kind of sdist the mock JFrog fixture uses:
// a .tar.gz with PKG-INFO containing Name/Version the migration reads.
func pythonSdistTarball(t *testing.T, name, version string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	pkgInfo := fmt.Sprintf(
		"Metadata-Version: 2.1\r\n"+
			"Name: %s\r\n"+
			"Version: %s\r\n"+
			"Summary: Mock Python package for migration testing\r\n"+
			"Classifier: Development Status :: 5 - Production/Stable\r\n"+
			"Classifier: License :: OSI Approved :: Apache Software License\r\n"+
			"\r\n"+
			"A HTTP library for Python.\r\n",
		name, version,
	)
	hdr := &tar.Header{
		Name: fmt.Sprintf("%s-%s/PKG-INFO", name, version),
		Mode: 0o644,
		Size: int64(len(pkgInfo)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write([]byte(pkgInfo)); err != nil {
		t.Fatalf("tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func assertMapString(t *testing.T, m map[string]interface{}, key, want string) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%q: want string, got %T (%v)", key, v, v)
	}
	if s != want {
		t.Errorf("%q = %q, want %q", key, s, want)
	}
}

func assertMapStringSlice(t *testing.T, m map[string]interface{}, key string, want []string) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	got, ok := v.([]string)
	if !ok {
		t.Fatalf("%q: want []string, got %T (%v)", key, v, v)
	}
	if len(got) != len(want) {
		t.Fatalf("%q: got %d values %v, want %d values %v", key, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q[%d] = %q, want %q", key, i, got[i], want[i])
		}
	}
}
