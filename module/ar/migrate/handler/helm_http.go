package handler

// helmHTTPHandler registers HELM_HTTP. Per the §2.4 matrix
// (docs/migration-type-handler-factory-plan.md): MigrationLevel =
// LevelPackageFile (per D-04), SkipLevel = LevelVersion (VersionExists-based
// skip check, package.go:111).

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type helmHTTPHandler struct {
	base
}

func (helmHTTPHandler) Type() types.ArtifactType { return types.HELM_HTTP }

func (helmHTTPHandler) MigrationLevel() Level { return LevelPackageFile }

func (helmHTTPHandler) SkipLevel() Level { return LevelVersion }

func init() {
	if err := Register(helmHTTPHandler{}); err != nil {
		return
	}
}
