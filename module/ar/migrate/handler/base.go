package handler

import (
	"context"
	"errors"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

var ErrNotImplemented = errors.New("handler: method not implemented")

type base struct{}

// Type has no sensible default; concrete handlers always override it.
func (base) Type() types.ArtifactType { return types.ArtifactType("") }

func (base) MigrationLevel() Level { return LevelNone }

func (base) SkipLevel() Level { return LevelNone }

func (base) EnumeratePackages(ctx context.Context, ec *EnumContext) ([]types.Package, error) {
	return nil, ErrNotImplemented
}

func (base) EnumerateVersions(ctx context.Context, ec *EnumContext) ([]types.Version, error) {
	return nil, ErrNotImplemented
}

func (base) SelectVersionFiles(ctx context.Context, ec *EnumContext) ([]*types.File, error) {
	return nil, ErrNotImplemented
}

func (base) PackageSkip(ctx context.Context, sc *SkipContext) (SkipResult, error) {
	return SkipResult{}, ErrNotImplemented
}

func (base) VersionSkip(ctx context.Context, sc *SkipContext) (bool, error) {
	return false, ErrNotImplemented
}

func (base) FileSkip(ctx context.Context, sc *SkipContext) (bool, error) {
	return false, ErrNotImplemented
}

func (base) MigratePackage(ctx context.Context, mc *MigrateContext) error {
	return ErrNotImplemented
}

func (base) MigrateVersion(ctx context.Context, mc *MigrateContext) error {
	return ErrNotImplemented
}

func (base) MigrateFile(ctx context.Context, mc *MigrateContext) error {
	return ErrNotImplemented
}

var _ Handler = base{}
