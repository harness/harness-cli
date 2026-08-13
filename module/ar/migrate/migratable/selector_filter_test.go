package migratable

import (
	"context"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// TestVersionMigrateFileSelector verifies that Version.Migrate honors the
// opt-in file selector (packageFilters[].files): only files named in the
// selector are migrated, and files not named are skipped.
func TestVersionMigrateFileSelector(t *testing.T) {
	t.Run("WithFileSelector", func(t *testing.T) {
		// Build a Version job with a mapping that filters files: only "keep.txt" is allowed
		src := &indexFakeSrc{content: map[string][]byte{
			"/keep.txt": []byte("kept"),
			"/drop.txt": []byte("dropped"),
		}}
		dest := &indexFakeDest{}
		stats := &types.TransferStats{}

		node := genericFileTree("keep.txt", "drop.txt")
		mapping := &types.RegistryMapping{
			PackageFilters: []types.PackageSelector{
				{Package: "my-package", Files: []string{"keep.txt"}},
			},
		}

		job := &Version{
			srcRegistry:   "src-reg",
			destRegistry:  "dst-reg",
			srcAdapter:    src,
			destAdapter:   dest,
			artifactType:  types.GENERIC,
			logger:        zerolog.Nop(),
			pkg:           types.Package{Name: "my-package"},
			version:       types.Version{Name: "1.0.0"},
			node:          node,
			stats:         stats,
			mapping:       mapping,
			config:        &types.Config{Concurrency: 1, DryRun: false, Overwrite: false},
			existingIndex: nil,
		}

		if err := job.Migrate(context.Background()); err != nil {
			t.Fatalf("Migrate() failed: %v", err)
		}

		// Only keep.txt should have been uploaded
		if len(dest.uploaded) != 1 || dest.uploaded[0] != "keep.txt" {
			t.Errorf("dest uploads = %v, want [keep.txt] (drop.txt filtered out)", dest.uploaded)
		}
	})

	t.Run("NoMappingUploadsAll", func(t *testing.T) {
		// Verify behavior-preserving: with NO mapping, BOTH files upload
		src := &indexFakeSrc{content: map[string][]byte{
			"/a.txt": []byte("a"),
			"/b.txt": []byte("b"),
		}}
		dest := &indexFakeDest{}
		stats := &types.TransferStats{}

		node := genericFileTree("a.txt", "b.txt")

		job := &Version{
			srcRegistry:   "src-reg",
			destRegistry:  "dst-reg",
			srcAdapter:    src,
			destAdapter:   dest,
			artifactType:  types.GENERIC,
			logger:        zerolog.Nop(),
			pkg:           types.Package{Name: "my-package"},
			version:       types.Version{Name: "1.0.0"},
			node:          node,
			stats:         stats,
			mapping:       nil,
			config:        &types.Config{Concurrency: 1, DryRun: false, Overwrite: false},
			existingIndex: nil,
		}

		if err := job.Migrate(context.Background()); err != nil {
			t.Fatalf("Migrate() failed: %v", err)
		}

		if len(dest.uploaded) != 2 {
			t.Errorf("dest uploads = %v, want both files uploaded (no filter)", dest.uploaded)
		}
	})
}

// TestPackageMigrateVersionSelector verifies that buildVersionJobs honors the
// opt-in version selector (packageFilters[].versions): only versions named in
// the selector produce jobs, and versions not named are skipped.
func TestPackageMigrateVersionSelector(t *testing.T) {
	t.Run("VersionSelectorFiltersPython", func(t *testing.T) {
		// Set up a PYTHON package with 3 versions, but the selector only allows "2.28.0"
		versions := []types.Version{
			{Pkg: "requests", Name: "0.1.1", Path: "/requests/0.1.1/requests-0.1.1.tar.gz"},
			{Pkg: "requests", Name: "2.28.0", Path: "/requests/2.28.0/requests-2.28.0.tar.gz"},
			{Pkg: "requests", Name: "2.29.0", Path: "/requests/2.29.0/requests-2.29.0.tar.gz"},
		}

		// Build a tree that contains all three versions' files
		files := []types.File{
			{Name: "requests-0.1.1.tar.gz", Uri: "/requests/0.1.1/requests-0.1.1.tar.gz", Size: 100},
			{Name: "requests-2.28.0.tar.gz", Uri: "/requests/2.28.0/requests-2.28.0.tar.gz", Size: 4096},
			{Name: "requests-2.29.0.tar.gz", Uri: "/requests/2.29.0/requests-2.29.0.tar.gz", Size: 4200},
		}
		node := tree.TransformToTree(files)

		mapping := &types.RegistryMapping{
			PackageFilters: []types.PackageSelector{
				{Package: "requests", Versions: []string{"2.28.0"}},
			},
		}

		pkg := &Package{
			artifactType:   types.PYTHON,
			pkg:            types.Package{Name: "requests"},
			node:           node,
			unfilteredNode: node,
			mapping:        mapping,
			logger:         zerolog.Nop(),
		}

		jobs := pkg.buildVersionJobs(versions, zerolog.Nop())

		// Should produce jobs only for version 2.28.0
		if len(jobs) != 1 {
			t.Fatalf("expected 1 job (only 2.28.0 selected), got %d", len(jobs))
		}

		// Verify the job is for the selected version
		vJob, ok := jobs[0].(*Version)
		if !ok {
			t.Fatalf("expected job to be *Version, got %T", jobs[0])
		}
		if vJob.version.Name != "2.28.0" {
			t.Errorf("expected job for version 2.28.0, got %s", vJob.version.Name)
		}
	})

	t.Run("NoSelectorAllowsAllVersions", func(t *testing.T) {
		// Verify behavior-preserving: with NO version selector, all versions pass through
		versions := []types.Version{
			{Pkg: "requests", Name: "0.1.1", Path: "/requests/0.1.1/requests-0.1.1.tar.gz"},
			{Pkg: "requests", Name: "2.28.0", Path: "/requests/2.28.0/requests-2.28.0.tar.gz"},
		}

		files := []types.File{
			{Name: "requests-0.1.1.tar.gz", Uri: "/requests/0.1.1/requests-0.1.1.tar.gz", Size: 100},
			{Name: "requests-2.28.0.tar.gz", Uri: "/requests/2.28.0/requests-2.28.0.tar.gz", Size: 4096},
		}
		node := tree.TransformToTree(files)

		pkg := &Package{
			artifactType:   types.PYTHON,
			pkg:            types.Package{Name: "requests"},
			node:           node,
			unfilteredNode: node,
			mapping:        nil,
			logger:         zerolog.Nop(),
		}

		jobs := pkg.buildVersionJobs(versions, zerolog.Nop())

		// Should produce jobs for all versions
		if len(jobs) != 2 {
			t.Fatalf("expected 2 jobs (no filter), got %d", len(jobs))
		}
	})

	t.Run("EmptyVersionsListAllowsAll", func(t *testing.T) {
		// Verify that a selector with an empty Versions list allows all versions
		versions := []types.Version{
			{Pkg: "requests", Name: "0.1.1", Path: "/requests/0.1.1/requests-0.1.1.tar.gz"},
			{Pkg: "requests", Name: "2.28.0", Path: "/requests/2.28.0/requests-2.28.0.tar.gz"},
		}

		files := []types.File{
			{Name: "requests-0.1.1.tar.gz", Uri: "/requests/0.1.1/requests-0.1.1.tar.gz", Size: 100},
			{Name: "requests-2.28.0.tar.gz", Uri: "/requests/2.28.0/requests-2.28.0.tar.gz", Size: 4096},
		}
		node := tree.TransformToTree(files)

		mapping := &types.RegistryMapping{
			PackageFilters: []types.PackageSelector{
				{Package: "requests", Versions: []string{}}, // Empty Versions list
			},
		}

		pkg := &Package{
			artifactType:   types.PYTHON,
			pkg:            types.Package{Name: "requests"},
			node:           node,
			unfilteredNode: node,
			mapping:        mapping,
			logger:         zerolog.Nop(),
		}

		jobs := pkg.buildVersionJobs(versions, zerolog.Nop())

		// Should produce jobs for all versions
		if len(jobs) != 2 {
			t.Fatalf("expected 2 jobs (empty Versions list = all versions), got %d", len(jobs))
		}
	})
}
