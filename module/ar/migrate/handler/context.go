package handler

import (
	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type JobDeps struct {
	Src, Dest    adapter.Adapter
	SrcRegistry  string
	DestRegistry string
	SrcPkgHost   string
	ArtifactType types.ArtifactType
	Mapping      *types.RegistryMapping
	Config       *types.Config
	Registry     types.RegistryInfo
	Stats        *types.TransferStats
	DryRun       *types.DryRunStats
	// Cache *types.ExistingIndex is introduced in Arc B (Phase B1); the type
	// does not exist yet in A0, so the field is intentionally omitted here.
}

type EnumContext struct {
	*JobDeps
	Pkg     types.Package
	Version types.Version
	Node    *types.TreeNode
}

type SkipContext struct {
	*JobDeps
	Pkg     types.Package
	Version types.Version
	File    *types.File
}

type MigrateContext struct {
	*JobDeps
	Pkg     types.Package
	Version types.Version
	File    *types.File
	Node    *types.TreeNode
}

// SkipResult is intentionally minimal for A0; it will grow (e.g. per-tag /
// per-version granularity) when skip logic is extracted in Arc A4 / Arc B.
type SkipResult struct {
	Skip bool
}
