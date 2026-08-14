package migratable

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// registryFakeSrc returns a fixed file list and fixed package list.
type registryFakeSrc struct {
	noopAdapter
	files []types.File
	pkgs  []types.Package
}

func (s *registryFakeSrc) GetFiles(_ string) ([]types.File, error) { return s.files, nil }
func (s *registryFakeSrc) GetPackages(_ string, _ types.ArtifactType, _ *types.TreeNode) ([]types.Package, error) {
	return s.pkgs, nil
}

func newRegistryJob(src *registryFakeSrc, at types.ArtifactType) *Registry {
	return &Registry{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  &noopAdapter{},
		artifactType: at,
		logger:       zerolog.Nop(),
		stats:        &types.TransferStats{},
		mapping:      &types.RegistryMapping{},
		config:       &types.Config{Concurrency: 1},
	}
}

// TestRegistryMigrateZeroPackagesGuardFires checks that Migrate errors when
// the source has files but GetPackages resolves zero packages and no filters
// are active (misconfigured artifactType scenario).
func TestRegistryMigrateZeroPackagesGuardFires(t *testing.T) {
	src := &registryFakeSrc{
		files: []types.File{{Name: "some-file.tgz", Uri: "/some-file.tgz"}},
		pkgs:  []types.Package{}, // adapter returns no packages
	}
	job := newRegistryJob(src, types.TERRAFORM)

	err := job.Migrate(context.Background())
	if err == nil {
		t.Fatal("expected error for 0 packages with non-empty source, got nil")
	}
	if !strings.Contains(err.Error(), "resolved 0 packages") {
		t.Errorf("error message %q does not mention '0 packages'", err.Error())
	}
}

// TestRegistryMigrateZeroPackagesGuardSilentWhenEmpty checks that Migrate does
// NOT error when the source registry itself is empty (no files, no packages).
func TestRegistryMigrateZeroPackagesGuardSilentWhenEmpty(t *testing.T) {
	src := &registryFakeSrc{files: nil, pkgs: nil}
	job := newRegistryJob(src, types.TERRAFORM)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("unexpected error for empty registry: %v", err)
	}
}

// TestRegistryMigrateComposerFiltersReducedToZero warns when legacy-style
// includePatterns (zip basename globs) no longer match vendor/package names.
func TestRegistryMigrateComposerFiltersReducedToZero(t *testing.T) {
	var logBuf bytes.Buffer
	src := &registryFakeSrc{
		files: []types.File{{Name: "harness-migtest-1.0.0.zip", Uri: "/harness-migtest/harness-migtest-1.0.0.zip"}},
		pkgs:  []types.Package{{Name: "harness/migtest", Path: "/"}},
	}
	job := newRegistryJob(src, types.COMPOSER)
	job.logger = zerolog.New(&logBuf)
	job.mapping = &types.RegistryMapping{
		IncludePatterns: []string{"harness-migtest*"},
	}

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := logBuf.String()
	if !strings.Contains(out, "Composer package filters reduced 1 package(s) to 0") {
		t.Fatalf("expected filter-to-zero warning in logs, got: %s", out)
	}
	if !strings.Contains(out, "vendor/package") {
		t.Fatalf("expected Composer naming hint in logs, got: %s", out)
	}
}

func TestIndexApplicable(t *testing.T) {
	applicable := []types.ArtifactType{
		types.GENERIC, types.RAW, types.PYTHON, types.NUGET,
		types.DART, types.PUPPET, types.RUBY, types.NPM, types.MAVEN,
	}
	for _, at := range applicable {
		if !indexApplicable(at) {
			t.Errorf("indexApplicable(%q) = false, want true", at)
		}
	}

	notApplicable := []types.ArtifactType{
		types.TERRAFORM, types.DOCKER, types.HELM, types.GO,
		types.RPM, types.DEBIAN, types.CONDA, types.COMPOSER,
		types.SWIFT, types.CONAN,
	}
	for _, at := range notApplicable {
		if indexApplicable(at) {
			t.Errorf("indexApplicable(%q) = true, want false", at)
		}
	}
}
