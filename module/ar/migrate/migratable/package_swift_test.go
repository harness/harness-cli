package migratable

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

type swiftFakeSrc struct {
	noopAdapter
	content        map[string][]byte
	failURI        string
	versions       []types.Version
	getVersionsErr error
}

func (s *swiftFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	if uri == s.failURI {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

func (s *swiftFakeSrc) GetVersions(_ types.Package, _ *types.TreeNode, _, pkg string, artifactType types.ArtifactType) ([]types.Version, error) {
	if artifactType != types.SWIFT {
		return nil, fmt.Errorf("unexpected artifact type %s", artifactType)
	}
	if s.getVersionsErr != nil {
		return nil, s.getVersionsErr
	}
	return s.versions, nil
}

type swiftFakeDest struct {
	noopAdapter
	uploads  []string
	skipName string
	failName string
}

func (d *swiftFakeDest) UploadFile(
	_ string,
	_ io.ReadCloser,
	f *types.File,
	_ http.Header,
	artifactName string,
	version string,
	_ types.ArtifactType,
	_ map[string]interface{},
) error {
	key := artifactName + "@" + version
	if f != nil && f.Name != "" {
		key = f.Name
	}
	if key == d.failName {
		return fmt.Errorf("upload rejected")
	}
	if key == d.skipName || artifactName+"@"+version == d.skipName {
		return types.ErrArtifactAlreadyExists
	}
	d.uploads = append(d.uploads, key)
	return nil
}

func newSwiftJob(src *swiftFakeSrc, dest *swiftFakeDest, stats *types.TransferStats) *Package {
	return &Package{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.SWIFT,
		logger:       zerolog.Nop(),
		pkg: types.Package{
			Name: "myscope.harness",
			Path: "/",
		},
		node:   &types.TreeNode{Name: "/", Key: "/"},
		stats:  stats,
		config: &types.Config{Concurrency: 1, DryRun: false, Overwrite: false},
	}
}

func TestMigrateSwiftSuccess(t *testing.T) {
	src := &swiftFakeSrc{
		content: map[string][]byte{
			"/myscope/harness/1.0.0/harness-1.0.0.zip": []byte("v1"),
		},
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
		},
	}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift: %v", err)
	}
	if len(dest.uploads) != 1 || dest.uploads[0] != "harness-1.0.0.zip" {
		t.Fatalf("uploads = %v, want [harness-1.0.0.zip]", dest.uploads)
	}
}

func TestMigrateSwiftMultiVersion(t *testing.T) {
	src := &swiftFakeSrc{
		content: map[string][]byte{
			"/myscope/harness/1.0.0/harness-1.0.0.zip": []byte("v1"),
			"/myscope/harness/1.0.1/harness-1.0.1.zip": []byte("v2"),
			"/myscope/harness/2.0.0/harness-2.0.0.zip": []byte("v3"),
		},
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
			{Path: "/myscope/harness/1.0.1/harness-1.0.1.zip", Name: "1.0.1", Size: 703652},
			{Path: "/myscope/harness/2.0.0/harness-2.0.0.zip", Name: "2.0.0", Size: 5000},
		},
	}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift: %v", err)
	}
	if len(dest.uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d: %v", len(dest.uploads), dest.uploads)
	}
	if len(stats.FileStats) != 3 || stats.FileStats[0].Status != types.StatusSuccess {
		t.Fatalf("stats = %+v, want 3 success", stats.FileStats)
	}
}

func TestMigrateSwiftSkipsExisting(t *testing.T) {
	src := &swiftFakeSrc{
		content: map[string][]byte{
			"/myscope/harness/1.0.0/harness-1.0.0.zip": []byte("v1"),
		},
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
		},
	}
	dest := &swiftFakeDest{skipName: "harness-1.0.0.zip"}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift: %v", err)
	}
	if len(dest.uploads) != 0 {
		t.Fatalf("skipped package must not upload, got %+v", dest.uploads)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusSkip {
		t.Fatalf("stats = %+v, want 1 skip", stats.FileStats)
	}
}

func TestMigrateSwiftDryRun(t *testing.T) {
	src := &swiftFakeSrc{
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
			{Path: "/myscope/harness/1.0.1/harness-1.0.1.zip", Name: "1.0.1", Size: 703652},
		},
	}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	job.config = &types.Config{DryRun: true}

	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift (dry-run): %v", err)
	}
	if len(dest.uploads) != 0 {
		t.Fatalf("dry-run must not upload, got %+v", dest.uploads)
	}
}

func TestMigrateSwiftDownloadFailure(t *testing.T) {
	src := &swiftFakeSrc{
		failURI: "/myscope/harness/1.0.0/harness-1.0.0.zip",
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
		},
	}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift should not abort on download failure: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusFail {
		t.Fatalf("stats = %+v, want 1 fail", stats.FileStats)
	}
}

func TestMigrateSwiftContinuesAfterDownloadFailure(t *testing.T) {
	src := &swiftFakeSrc{
		failURI: "/myscope/harness/1.0.0/harness-1.0.0.zip",
		content: map[string][]byte{
			"/myscope/harness/1.0.1/harness-1.0.1.zip": []byte("v2"),
		},
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
			{Path: "/myscope/harness/1.0.1/harness-1.0.1.zip", Name: "1.0.1", Size: 703652},
		},
	}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift: %v", err)
	}
	if len(dest.uploads) != 1 || dest.uploads[0] != "harness-1.0.1.zip" {
		t.Fatalf("uploads = %v, want [harness-1.0.1.zip]", dest.uploads)
	}
	if len(stats.FileStats) != 2 {
		t.Fatalf("stats = %+v, want 2 entries", stats.FileStats)
	}
	if stats.FileStats[0].Status != types.StatusFail || stats.FileStats[1].Status != types.StatusSuccess {
		t.Fatalf("stats = %+v, want fail then success", stats.FileStats)
	}
}

func TestMigrateSwiftGetVersionsError(t *testing.T) {
	src := &swiftFakeSrc{getVersionsErr: fmt.Errorf("source unavailable")}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err == nil {
		t.Fatal("expected GetVersions error")
	}
}

func TestMigrateSwiftNoVersionsFound(t *testing.T) {
	src := &swiftFakeSrc{versions: nil}
	dest := &swiftFakeDest{}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift should not return error: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusFail {
		t.Fatalf("stats = %+v, want 1 fail for empty version list", stats.FileStats)
	}
}

func TestMigrateSwiftUploadFailure(t *testing.T) {
	src := &swiftFakeSrc{
		content: map[string][]byte{
			"/myscope/harness/1.0.0/harness-1.0.0.zip": []byte("v1"),
		},
		versions: []types.Version{
			{Path: "/myscope/harness/1.0.0/harness-1.0.0.zip", Name: "1.0.0", Size: 1850},
		},
	}
	dest := &swiftFakeDest{failName: "harness-1.0.0.zip"}
	stats := &types.TransferStats{}

	job := newSwiftJob(src, dest, stats)
	if err := job.migrateSwift(context.Background()); err != nil {
		t.Fatalf("migrateSwift should not return error on upload failure: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusFail {
		t.Fatalf("stats = %+v, want 1 fail", stats.FileStats)
	}
}
