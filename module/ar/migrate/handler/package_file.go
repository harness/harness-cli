package handler

// packageFileHandler registers RPM, DEBIAN, CONDA, COMPOSER, SWIFT. Per the
// §2.4 matrix (docs/migration-type-handler-factory-plan.md) and decision
// D-01: MigrationLevel = LevelPackageFile (per D-04), SkipLevel =
// LevelPackageFile (the declared existence granularity, distinct from the
// "(none; server)" note in the "Skip check today" column which describes only
// the current implementation, not the declared granularity).

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type packageFileHandler struct {
	base
	typ types.ArtifactType
}

func (h packageFileHandler) Type() types.ArtifactType { return h.typ }

func (h packageFileHandler) MigrationLevel() Level { return LevelPackageFile }

func (h packageFileHandler) SkipLevel() Level { return LevelPackageFile }

func init() {
	artifactTypes := []types.ArtifactType{types.RPM, types.DEBIAN, types.CONDA, types.COMPOSER, types.SWIFT}
	for _, t := range artifactTypes {
		if err := Register(packageFileHandler{typ: t}); err != nil {
			return
		}
	}
}
