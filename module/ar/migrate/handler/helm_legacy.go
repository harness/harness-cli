package handler

// helmLegacyHandler registers HELM_LEGACY. Per the §2.4 matrix
// (docs/migration-type-handler-factory-plan.md): MigrationLevel =
// LevelPackageFile (per D-04), SkipLevel = LevelVersion (VersionExists-based
// skip check, package.go:111).

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type helmLegacyHandler struct {
	base
}

func (helmLegacyHandler) Type() types.ArtifactType { return types.HELM_LEGACY }

func (helmLegacyHandler) MigrationLevel() Level { return LevelPackageFile }

func (helmLegacyHandler) SkipLevel() Level { return LevelVersion }

func init() {
	if err := Register(helmLegacyHandler{}); err != nil {
		return
	}
}
