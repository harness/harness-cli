package adapter

import (
	"context"
	"errors"
	"fmt"
	"harness/clients/ar"
	"harness/module/ar/migrate/types"
)

type Adapter interface {
	ValidateCredentials() (bool, error)
	GetRegistry(registry string) (interface{}, error)
	CreateRegistryIfDoesntExist(registry string) (bool, error)
	GetPackages(registry string)
	GetVersions(registry, pkg string)
	GetFiles(registry, pkg, version string)
}

var registry = map[types.RegistryType]Factory{}

type Factory interface {
	Create(ctx context.Context, config types.RegistryConfig) (Adapter, error)
}

// RegisterFactory registers one adapter factory to the registry.
func RegisterFactory(t types.RegistryType, factory Factory) error {
	if len(t) == 0 {
		return errors.New("invalid type")
	}
	if factory == nil {
		return errors.New("empty adapter factory")
	}

	if _, exist := registry[t]; exist {
		return fmt.Errorf("adapter factory for %s already exists", t)
	}
	registry[t] = factory
	return nil
}

// GetFactory gets the adapter factory by the specified name.
func GetFactory(t types.RegistryType) (Factory, error) {
	factory, exist := registry[t]
	if !exist {
		return nil, fmt.Errorf("adapter factory for %s not found", t)
	}
	return factory, nil
}

func GetAdapter(ctx context.Context, cfg types.RegistryConfig, client *ar.Client) (Adapter, error) {
	factory, err := GetFactory(cfg.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to get adapter factory: %v", err)
	}
	adapter, err := factory.Create(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %v", err)
	}
	return adapter, nil
}
