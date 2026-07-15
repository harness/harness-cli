package handler

// ociHandler registers the OCI-backed types (DOCKER, HELM). Per the §2.4
// matrix (docs/migration-type-handler-factory-plan.md) and decision D-04,
// OCI types migrate/skip at per-tag granularity: MigrationLevel = LevelTag,
// SkipLevel = LevelTag.

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type ociHandler struct {
	base
	typ types.ArtifactType
}

func (h ociHandler) Type() types.ArtifactType { return h.typ }

func (h ociHandler) MigrationLevel() Level { return LevelTag }

func (h ociHandler) SkipLevel() Level { return LevelTag }

func init() {
	for _, t := range []types.ArtifactType{types.DOCKER, types.HELM} {
		if err := Register(ociHandler{typ: t}); err != nil {
			return
		}
	}
}
