package migratable

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// TestMigrateRPMFileStatReflectsArtifactSize mirrors a customer's RPM migration
// after JFrog enumeration via repodata/primary.xml:
//
//  1. extractRPMPackages sets pkg.Size from <size package="0"/> (mock fixture).
//  2. migrateRPM downloads the real RPM bytes from JFrog.
//  3. TransferStats should record the artifact size customers see in logs.
//
// Today migrateRPM copies enumerated pkg.Size into FileStat, so stats show 0.00B
// even when the downloaded RPM is non-empty and upload succeeds.
func TestMigrateRPMFileStatReflectsArtifactSize(t *testing.T) {
	t.Helper()

	const rpmURI = "/mockpkg-1.0.0-1.x86_64.rpm"
	rpmBytes := []byte("rpm-artifact-bytes-for-size-test")

	src := &rpmFakeSrc{content: map[string][]byte{rpmURI: rpmBytes}}
	dest := &rpmFakeDest{}
	stats := &types.TransferStats{}

	job := newRPMJob(src, dest, types.Package{
		Registry: "rpm-local",
		Name:     "mockpkg-1.0.0-1.x86_64.rpm", // filename from location.href today
		URL:      rpmURI,
		Size:     0,                               // from <size package="0"/> in primary.xml
	}, stats)

	if err := job.migrateRPM(context.Background()); err != nil {
		t.Fatalf("migrateRPM: %v", err)
	}
	if len(stats.FileStats) != 1 {
		t.Fatalf("expected 1 FileStat, got %d: %+v", len(stats.FileStats), stats.FileStats)
	}

	stat := stats.FileStats[0]
	if stat.Status != types.StatusSuccess {
		t.Fatalf("expected Success, got %s (err=%q)", stat.Status, stat.Error)
	}
	if stat.Size != int64(len(rpmBytes)) {
		t.Errorf("FileStat.Size = %d, want %d (downloaded RPM bytes, not enumerated pkg.Size=%d from primary.xml)",
			stat.Size, len(rpmBytes), job.pkg.Size)
	}
}

func newRPMJob(src adp.Adapter, dest adp.Adapter, pkg types.Package, stats *types.TransferStats) *Package {
	return &Package{
		srcRegistry:  "rpm-local",
		destRegistry: "dst-rpm",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.RPM,
		logger:       zerolog.Nop(),
		pkg:          pkg,
		stats:        stats,
		config:       &types.Config{},
	}
}

type rpmFakeSrc struct {
	noopAdapter
	content map[string][]byte
}

func (s *rpmFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	return io.NopCloser(bytes.NewReader(b)), http.Header{}, nil
}

type rpmFakeDest struct {
	noopAdapter
}

func (d *rpmFakeDest) UploadFile(
	_ string,
	file io.ReadCloser,
	f *types.File,
	_ http.Header,
	_ string,
	_ string,
	artifactType types.ArtifactType,
	_ map[string]interface{},
) error {
	if artifactType != types.RPM {
		return fmt.Errorf("unexpected artifact type %s", artifactType)
	}
	if file != nil {
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
	}
	_ = f
	return nil
}
