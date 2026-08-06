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

type composerFakeSrc struct {
	noopAdapter
	content       map[string][]byte
	failURI       string
	versions      []types.Version
	getVersionsErr error
}

func (s *composerFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	if uri == s.failURI {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

func (s *composerFakeSrc) GetVersions(_ types.Package, _ *types.TreeNode, _, pkg string, artifactType types.ArtifactType) ([]types.Version, error) {
	if artifactType != types.COMPOSER {
		return nil, fmt.Errorf("unexpected artifact type %s", artifactType)
	}
	if s.getVersionsErr != nil {
		return nil, s.getVersionsErr
	}
	return s.versions, nil
}

type composerFakeDest struct {
	noopAdapter
	uploads  []string
	skipName string
	failName string
}

func (d *composerFakeDest) UploadFile(
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

func newComposerJob(src *composerFakeSrc, dest *composerFakeDest, stats *types.TransferStats) *Package {
	return &Package{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.COMPOSER,
		logger:       zerolog.Nop(),
		pkg: types.Package{
			Name: "harness/migtest",
			Path: "/",
		},
		node:   &types.TreeNode{Name: "/", Key: "/"},
		stats:  stats,
		config: &types.Config{Concurrency: 1, DryRun: false, Overwrite: false},
	}
}

func TestMigrateComposerSuccess(t *testing.T) {
	src := &composerFakeSrc{
		content: map[string][]byte{
			"/harness-migtest/harness-migtest-1.0.0.zip": []byte("v1"),
		},
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer: %v", err)
	}
	if len(dest.uploads) != 1 || dest.uploads[0] != "harness-migtest-1.0.0.zip" {
		t.Fatalf("uploads = %v, want [harness-migtest-1.0.0.zip]", dest.uploads)
	}
}

func TestMigrateComposerMultiVersion(t *testing.T) {
	src := &composerFakeSrc{
		content: map[string][]byte{
			"/harness-migtest/harness-migtest-1.0.0.zip": []byte("v1"),
			"/harness-migtest/harness-migtest-2.0.0.zip": []byte("v2"),
			"/harness-migtest/harness-migtest-3.0.0.zip": []byte("v3"),
		},
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
			{Path: "/harness-migtest/harness-migtest-2.0.0.zip", Name: "2.0.0", Size: 2},
			{Path: "/harness-migtest/harness-migtest-3.0.0.zip", Name: "3.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer: %v", err)
	}
	if len(dest.uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d: %v", len(dest.uploads), dest.uploads)
	}
	if len(stats.FileStats) != 3 || stats.FileStats[0].Status != types.StatusSuccess {
		t.Fatalf("stats = %+v, want 3 success", stats.FileStats)
	}
}

func TestMigrateComposerSkipsExisting(t *testing.T) {
	src := &composerFakeSrc{
		content: map[string][]byte{
			"/harness-migtest/harness-migtest-1.0.0.zip": []byte("v1"),
		},
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{skipName: "harness-migtest-1.0.0.zip"}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer: %v", err)
	}
	if len(dest.uploads) != 0 {
		t.Fatalf("skipped package must not upload, got %+v", dest.uploads)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusSkip {
		t.Fatalf("stats = %+v, want 1 skip", stats.FileStats)
	}
}

func TestMigrateComposerDryRun(t *testing.T) {
	src := &composerFakeSrc{
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
			{Path: "/harness-migtest/harness-migtest-2.0.0.zip", Name: "2.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	job.config = &types.Config{DryRun: true}

	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer (dry-run): %v", err)
	}
	if len(dest.uploads) != 0 {
		t.Fatalf("dry-run must not upload, got %+v", dest.uploads)
	}
}

func TestMigrateComposerDownloadFailure(t *testing.T) {
	src := &composerFakeSrc{
		failURI: "/harness-migtest/harness-migtest-1.0.0.zip",
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer should not abort on download failure: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusFail {
		t.Fatalf("stats = %+v, want 1 fail", stats.FileStats)
	}
}

func TestMigrateComposerContinuesAfterDownloadFailure(t *testing.T) {
	src := &composerFakeSrc{
		failURI: "/harness-migtest/harness-migtest-1.0.0.zip",
		content: map[string][]byte{
			"/harness-migtest/harness-migtest-2.0.0.zip": []byte("v2"),
		},
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
			{Path: "/harness-migtest/harness-migtest-2.0.0.zip", Name: "2.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer: %v", err)
	}
	if len(dest.uploads) != 1 || dest.uploads[0] != "harness-migtest-2.0.0.zip" {
		t.Fatalf("uploads = %v, want [harness-migtest-2.0.0.zip]", dest.uploads)
	}
	if len(stats.FileStats) != 2 {
		t.Fatalf("stats = %+v, want 2 entries", stats.FileStats)
	}
	if stats.FileStats[0].Status != types.StatusFail || stats.FileStats[1].Status != types.StatusSuccess {
		t.Fatalf("stats = %+v, want fail then success", stats.FileStats)
	}
}

func TestMigrateComposerGetVersionsError(t *testing.T) {
	src := &composerFakeSrc{getVersionsErr: fmt.Errorf("source unavailable")}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err == nil {
		t.Fatal("expected GetVersions error")
	}
}

func TestMigrateComposerNoVersionsFound(t *testing.T) {
	src := &composerFakeSrc{versions: nil}
	dest := &composerFakeDest{}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer should record failure in stats, not return error: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusFail {
		t.Fatalf("stats = %+v, want 1 fail for empty version list", stats.FileStats)
	}
}

func TestMigrateComposerUploadFailure(t *testing.T) {
	src := &composerFakeSrc{
		content: map[string][]byte{
			"/harness-migtest/harness-migtest-1.0.0.zip": []byte("v1"),
		},
		versions: []types.Version{
			{Path: "/harness-migtest/harness-migtest-1.0.0.zip", Name: "1.0.0", Size: 2},
		},
	}
	dest := &composerFakeDest{failName: "harness-migtest-1.0.0.zip"}
	stats := &types.TransferStats{}

	job := newComposerJob(src, dest, stats)
	if err := job.migrateComposer(context.Background()); err != nil {
		t.Fatalf("migrateComposer should not return error on upload failure: %v", err)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusFail {
		t.Fatalf("stats = %+v, want 1 fail", stats.FileStats)
	}
}
