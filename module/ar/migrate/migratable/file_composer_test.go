package migratable

import (
	"context"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// TestFileMigrateComposerUploadsZip is a regression guard for the COMPOSER
// version→file-job flow. COMPOSER is enumerated as one logical package whose
// versions are fanned out into file jobs; each file job must download the zip
// from the source and upload it to the destination (the backend derives the
// name/version from the zip's composer.json). If COMPOSER is ever dropped from
// File.Migrate's generic download/upload branch, the migration silently uploads
// nothing while reporting no error — the exact "reports success but HAR is
// empty" failure. This test fails fast in that case.
func TestFileMigrateComposerUploadsZip(t *testing.T) {
	file := &types.File{Name: "vendor-package-2.0.0.zip", Uri: "/vendor-package-2.0.0.zip", Size: 640}
	src := &fakeSrc{content: map[string][]byte{
		"/vendor-package-2.0.0.zip": []byte("composer-zip-bytes"),
	}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}

	job := &File{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.COMPOSER,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: "vendor-package"},
		version:      types.Version{Name: "2.0.0"},
		file:         file,
		stats:        stats,
		config:       &types.Config{},
	}

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("File.Migrate(COMPOSER) returned err: %v", err)
	}

	if len(dest.uploaded) != 1 || dest.uploaded[0] != "vendor-package-2.0.0.zip" {
		t.Fatalf("dest uploads = %v, want [vendor-package-2.0.0.zip] (COMPOSER file was not uploaded)", dest.uploaded)
	}
	if len(stats.FileStats) != 1 {
		t.Fatalf("expected 1 FileStat, got %d: %+v", len(stats.FileStats), stats.FileStats)
	}
	if stats.FileStats[0].Status != types.StatusSuccess {
		t.Errorf("expected Success stat, got %s (err=%q)", stats.FileStats[0].Status, stats.FileStats[0].Error)
	}
}

// TestFileMigrateComposerDryRunSkipsTransfer verifies the COMPOSER file job does
// not download or upload in dry-run mode.
func TestFileMigrateComposerDryRunSkipsTransfer(t *testing.T) {
	file := &types.File{Name: "vendor-package-2.0.0.zip", Uri: "/vendor-package-2.0.0.zip", Size: 640}
	src := &fakeSrc{content: map[string][]byte{
		"/vendor-package-2.0.0.zip": []byte("composer-zip-bytes"),
	}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}

	job := &File{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.COMPOSER,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: "vendor-package"},
		version:      types.Version{Name: "2.0.0"},
		file:         file,
		stats:        stats,
		config:       &types.Config{DryRun: true},
	}

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("File.Migrate(COMPOSER dry-run) returned err: %v", err)
	}
	if len(dest.uploaded) != 0 {
		t.Errorf("dry-run must not upload, but dest received: %v", dest.uploaded)
	}
}
