package handler

// goHandler registers GO. Per the §2.4 matrix
// (docs/migration-type-handler-factory-plan.md) and decision D-02:
// MigrationLevel = LevelVersion (job tree terminates at the Version job's
// CreateVersion), SkipLevel = LevelVersion (declared granularity per D-02,
// independent of the "(none today)" implementation-status note).

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type goHandler struct {
	base
}

func (goHandler) Type() types.ArtifactType { return types.GO }

func (goHandler) MigrationLevel() Level { return LevelVersion }

func (goHandler) SkipLevel() Level { return LevelVersion }

func init() {
	if err := Register(goHandler{}); err != nil {
		return
	}
}
