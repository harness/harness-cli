package handler

// fileHandler registers the file-level types: GENERIC, RAW, MAVEN, PYTHON,
// NUGET, DART, PUPPET, NPM. Per the §2.4 matrix
// (docs/migration-type-handler-factory-plan.md), MigrationLevel = LevelFile
// for all of them. SkipLevel is LevelFile for GENERIC/RAW/PYTHON/NUGET/DART/
// PUPPET, but MAVEN and NPM declare LevelNone per decision D-03 (both are
// excluded from the client-side skip path at version.go:109; §2.4's "File"/
// "None (server-side)" labels for these two rows are superseded by the
// "(excluded today)" note and the CLAUDE.md type->level summary).

import (
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type fileHandler struct {
	base
	typ  types.ArtifactType
	skip Level
}

func (h fileHandler) Type() types.ArtifactType { return h.typ }

func (h fileHandler) MigrationLevel() Level { return LevelFile }

func (h fileHandler) SkipLevel() Level { return h.skip }

func init() {
	entries := []struct {
		typ  types.ArtifactType
		skip Level
	}{
		{types.GENERIC, LevelFile},
		{types.RAW, LevelFile},
		{types.MAVEN, LevelNone}, // D-03: excluded at version.go:109
		{types.PYTHON, LevelFile},
		{types.NUGET, LevelFile},
		{types.DART, LevelFile},
		{types.PUPPET, LevelFile},
		{types.NPM, LevelNone}, // D-03: excluded at version.go:109
	}

	for _, e := range entries {
		if err := Register(fileHandler{typ: e.typ, skip: e.skip}); err != nil {
			return
		}
	}
}
