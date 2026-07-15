package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type Level int

const (
	LevelNone Level = iota
	LevelTag
	LevelPackageFile
	LevelVersion
	LevelFile
)

type Handler interface {
	Type() types.ArtifactType

	MigrationLevel() Level
	SkipLevel() Level

	EnumeratePackages(ctx context.Context, ec *EnumContext) ([]types.Package, error)
	EnumerateVersions(ctx context.Context, ec *EnumContext) ([]types.Version, error)
	SelectVersionFiles(ctx context.Context, ec *EnumContext) ([]*types.File, error)

	PackageSkip(ctx context.Context, sc *SkipContext) (SkipResult, error)
	VersionSkip(ctx context.Context, sc *SkipContext) (bool, error)
	FileSkip(ctx context.Context, sc *SkipContext) (bool, error)

	MigratePackage(ctx context.Context, mc *MigrateContext) error
	MigrateVersion(ctx context.Context, mc *MigrateContext) error
	MigrateFile(ctx context.Context, mc *MigrateContext) error
}

var registry = map[types.ArtifactType]Handler{}

// Register registers one handler to the registry.
func Register(h Handler) error {
	if h == nil {
		return errors.New("empty handler")
	}

	t := h.Type()
	if len(t) == 0 {
		return errors.New("invalid type")
	}

	if _, exist := registry[t]; exist {
		return fmt.Errorf("handler for %s already exists", t)
	}
	registry[t] = h
	return nil
}

// Get gets the handler by the specified artifact type.
func Get(t types.ArtifactType) (Handler, error) {
	h, exist := registry[t]
	if !exist {
		return nil, fmt.Errorf("handler for %s not found", t)
	}
	return h, nil
}
