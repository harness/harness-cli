package handler

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestHandlerMatrix asserts every ArtifactType resolves to a registered
// handler declaring the exact MigrationLevel()/SkipLevel() from the §2.4
// matrix in docs/migration-type-handler-factory-plan.md (transcribed
// verbatim from the Phase A1 plan's objective, with decisions D-01..D-04
// applied). This table is an INDEPENDENT source of truth: it is NOT derived
// by reading the Task 1 handler files, so it verifies those files rather
// than restating them.
func TestHandlerMatrix(t *testing.T) {
	table := []struct {
		typ           types.ArtifactType
		wantMigration Level
		wantSkip      Level
	}{
		{types.DOCKER, LevelTag, LevelTag},
		{types.HELM, LevelTag, LevelTag},
		{types.HELM_LEGACY, LevelPackageFile, LevelVersion},
		{types.HELM_HTTP, LevelPackageFile, LevelVersion},
		{types.GO, LevelVersion, LevelVersion},
		{types.GENERIC, LevelFile, LevelFile},
		{types.RAW, LevelFile, LevelFile},
		{types.MAVEN, LevelFile, LevelNone},
		{types.PYTHON, LevelFile, LevelFile},
		{types.NUGET, LevelFile, LevelFile},
		{types.DART, LevelFile, LevelFile},
		{types.PUPPET, LevelFile, LevelFile},
		{types.NPM, LevelFile, LevelNone},
		{types.CONAN, LevelPackageFile, LevelPackageFile},
		{types.RPM, LevelPackageFile, LevelPackageFile},
		{types.DEBIAN, LevelPackageFile, LevelPackageFile},
		{types.CONDA, LevelPackageFile, LevelPackageFile},
		{types.COMPOSER, LevelPackageFile, LevelPackageFile},
		{types.SWIFT, LevelPackageFile, LevelPackageFile},
	}

	for _, row := range table {
		row := row
		t.Run(string(row.typ), func(t *testing.T) {
			h, err := Get(row.typ)
			if err != nil {
				t.Fatalf("Get(%s) returned unexpected error: %v", row.typ, err)
			}
			if h == nil {
				t.Fatalf("Get(%s) returned nil handler", row.typ)
			}
			if got := h.MigrationLevel(); got != row.wantMigration {
				t.Errorf("%s: MigrationLevel() = %v, want %v", row.typ, got, row.wantMigration)
			}
			if got := h.SkipLevel(); got != row.wantSkip {
				t.Errorf("%s: SkipLevel() = %v, want %v", row.typ, got, row.wantSkip)
			}
		})
	}

	if len(registry) != len(table) {
		t.Errorf("registry has %d entries, want exactly %d (table size)", len(registry), len(table))
	}

	wantTypes := make(map[types.ArtifactType]bool, len(table))
	for _, row := range table {
		wantTypes[row.typ] = true
	}
	for k := range registry {
		if !wantTypes[k] {
			t.Errorf("registry contains stray/unexpected type %q not present in the §2.4 matrix table", k)
		}
	}
}

// TestHandlerMatrixStranglerInvariant confirms that non-declarator methods on
// a registered handler still return ErrNotImplemented this phase (base is
// embedded; no per-type logic has been extracted into the handler yet).
func TestHandlerMatrixStranglerInvariant(t *testing.T) {
	h, err := Get(types.NUGET)
	if err != nil {
		t.Fatalf("Get(NUGET) returned unexpected error: %v", err)
	}

	if err := h.MigratePackage(nil, nil); err != ErrNotImplemented {
		t.Errorf("MigratePackage() = %v, want ErrNotImplemented", err)
	}
	if err := h.MigrateFile(nil, nil); err != ErrNotImplemented {
		t.Errorf("MigrateFile() = %v, want ErrNotImplemented", err)
	}
}
