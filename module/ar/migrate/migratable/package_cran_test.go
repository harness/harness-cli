package migratable

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/rs/zerolog"
)

type cranFakeSrc struct {
	noopAdapter
	content map[string][]byte
}

func (s *cranFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

type cranFakeDest struct {
	noopAdapter
	uploadedUris []string
	uploadErr    error
	exists       bool
	existsErr    error
	headURI      string
}

func (d *cranFakeDest) UploadFile(
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
	if d.uploadErr != nil {
		return d.uploadErr
	}
	d.uploadedUris = append(d.uploadedUris, f.Uri)
	return nil
}

func (d *cranFakeDest) FileExists(
	_ context.Context,
	_, _, _ string,
	file *types.File,
	_ types.ArtifactType,
) (bool, error) {
	if file != nil {
		d.headURI = file.Uri
	}
	return d.exists, d.existsErr
}

func cranFileTree(uris ...string) *types.TreeNode {
	files := make([]types.File, 0, len(uris))
	for _, u := range uris {
		files = append(files, types.File{Name: path.Base(u), Uri: u, Size: 10})
	}
	return tree.TransformToTree(files)
}

func filesForPackage(node *types.TreeNode, pkgName string) []types.File {
	all, err := tree.GetAllFiles(node)
	if err != nil {
		return nil
	}
	flat := make([]types.File, 0, len(all))
	for _, f := range all {
		if f != nil {
			flat = append(flat, *f)
		}
	}
	return util.BuildCranPackageFilesMap(flat)[pkgName]
}

func newCranPackageJob(src *cranFakeSrc, dest *cranFakeDest, node *types.TreeNode, stats *types.TransferStats) *Package {
	pkgName := "jsonlite"
	return &Package{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.CRAN,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: pkgName, Path: "/"},
		node:         node,
		files:        filesForPackage(node, pkgName),
		stats:        stats,
		config:       &types.Config{Concurrency: 1, DryRun: false, Overwrite: false},
		mapping:      &types.RegistryMapping{},
		registry:     types.RegistryInfo{Path: "cran-dest"},
	}
}

func TestPackageMigrateCRANRemapsArchivePath(t *testing.T) {
	srcURI := "/src/contrib/Archive/jsonlite/1.7.0/jsonlite_1.7.0.tar.gz"
	node := cranFileTree(srcURI)
	src := &cranFakeSrc{content: map[string][]byte{srcURI: []byte("pkg")}}
	dest := &cranFakeDest{}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if len(dest.uploadedUris) != 1 || dest.uploadedUris[0] != "src/contrib/jsonlite_1.7.0.tar.gz" {
		t.Errorf("uploaded = %v, want [src/contrib/jsonlite_1.7.0.tar.gz]", dest.uploadedUris)
	}
}

func TestPackageMigrateCRANUploadsAllPlatformsForSameVersion(t *testing.T) {
	srcTar := "/src/contrib/jsonlite_1.8.0.tar.gz"
	winZip := "/bin/windows/contrib/4.4/jsonlite_1.8.0.zip"
	node := cranFileTree(srcTar, winZip)
	src := &cranFakeSrc{content: map[string][]byte{
		srcTar: []byte("src"),
		winZip: []byte("win"),
	}}
	dest := &cranFakeDest{}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if len(dest.uploadedUris) != 2 {
		t.Fatalf("uploaded = %v, want 2 files for version 1.8.0", dest.uploadedUris)
	}
}

func TestPackageMigrateCRANSkipsIndexAndOtherPackages(t *testing.T) {
	node := cranFileTree(
		"/src/contrib/jsonlite_1.8.0.tar.gz",
		"/src/contrib/Archive/jsonlite/1.7.0/jsonlite_1.7.0.tar.gz",
		"/src/contrib/data.table_1.14.0.tar.gz",
		"/src/contrib/PACKAGES",
	)
	src := &cranFakeSrc{content: map[string][]byte{
		"/src/contrib/jsonlite_1.8.0.tar.gz": []byte("live"),
	}}
	dest := &cranFakeDest{}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if len(dest.uploadedUris) != 1 {
		t.Errorf("uploaded count = %d, want 1 (jsonlite 1.8.0 only)", len(dest.uploadedUris))
	}
}

func TestPackageMigrateCRANAlreadyExists(t *testing.T) {
	srcURI := "/src/contrib/jsonlite_1.8.0.tar.gz"
	node := cranFileTree(srcURI)
	src := &cranFakeSrc{content: map[string][]byte{srcURI: []byte("pkg")}}
	dest := &cranFakeDest{uploadErr: types.ErrArtifactAlreadyExists}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusSkip {
		t.Errorf("stat = %+v, want StatusSkip", stats.FileStats)
	}
	if stats.FileStats[0].Reason != types.SkipReasonAlreadyExists {
		t.Errorf("Reason = %q, want %q", stats.FileStats[0].Reason, types.SkipReasonAlreadyExists)
	}
}

func TestPackageMigrateCRANSkipsUnrecognizedPaths(t *testing.T) {
	srcURI := "/not/a/cran/path.txt"
	node := cranFileTree(srcURI)
	src := &cranFakeSrc{content: map[string][]byte{srcURI: []byte("x")}}
	dest := &cranFakeDest{}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if len(dest.uploadedUris) != 0 {
		t.Errorf("expected no uploads for unrecognized path, got %v", dest.uploadedUris)
	}
	if len(stats.FileStats) != 0 {
		t.Errorf("stats = %+v, want no entries for skipped unrecognized path", stats.FileStats)
	}
}

func TestPackageMigrateCRANSkipsWhenHeadExists(t *testing.T) {
	srcURI := "/src/contrib/Archive/jsonlite/1.7.0/jsonlite_1.7.0.tar.gz"
	node := cranFileTree(srcURI)
	src := &cranFakeSrc{content: map[string][]byte{srcURI: []byte("pkg")}}
	dest := &cranFakeDest{exists: true}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if dest.headURI != "src/contrib/jsonlite_1.7.0.tar.gz" {
		t.Errorf("HEAD uri = %q, want remapped contrib path", dest.headURI)
	}
	if len(dest.uploadedUris) != 0 {
		t.Errorf("expected no uploads after HEAD skip, got %v", dest.uploadedUris)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusSkip {
		t.Errorf("stat = %+v, want StatusSkip", stats.FileStats)
	}
	if len(stats.FileStats) == 1 && stats.FileStats[0].Reason != types.SkipReasonAlreadyExists {
		t.Errorf("Reason = %q, want %q", stats.FileStats[0].Reason, types.SkipReasonAlreadyExists)
	}
}

func TestPackageMigrateCRANProceedsWhenHeadErrors(t *testing.T) {
	srcURI := "/src/contrib/jsonlite_1.8.0.tar.gz"
	node := cranFileTree(srcURI)
	src := &cranFakeSrc{content: map[string][]byte{srcURI: []byte("pkg")}}
	dest := &cranFakeDest{existsErr: errors.New("head failed")}
	stats := &types.TransferStats{}

	job := newCranPackageJob(src, dest, node, stats)
	if err := job.migrateCran(context.Background()); err != nil {
		t.Fatalf("migrateCran() error: %v", err)
	}
	if len(dest.uploadedUris) != 1 {
		t.Errorf("expected upload after HEAD error, got %v", dest.uploadedUris)
	}
}
