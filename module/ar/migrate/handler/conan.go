package handler

// conanHandler registers CONAN. Per the §2.4 matrix
// (docs/migration-type-handler-factory-plan.md) and decision D-01:
// MigrationLevel = LevelPackageFile (per D-04), SkipLevel = LevelPackageFile
// (CONAN's "File-in-package" label maps to the LevelPackageFile taxonomy
// slot, sharing package-granularity/server-side-409 dedup semantics with the
// RPM family).

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type conanHandler struct {
	base
}

func (conanHandler) Type() types.ArtifactType { return types.CONAN }

func (conanHandler) MigrationLevel() Level { return LevelPackageFile }

func (conanHandler) SkipLevel() Level { return LevelPackageFile }

func init() {
	if err := Register(conanHandler{}); err != nil {
		return
	}
}
