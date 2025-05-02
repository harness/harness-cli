package jfrog

import (
	"context"
	adp "harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
)

func init() {
	adapterType := types.JFROG
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

// factory section
type factory struct {
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

type adapter struct {
	client *client
}

func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	return &adapter{
		client: newClient(&config),
	}, nil
}

func (a *adapter) ValidateCredentials() (bool, error)                        { return false, nil }
func (a *adapter) GetRegistry(registry string) (interface{}, error)          { return nil, nil }
func (a *adapter) CreateRegistryIfDoesntExist(registry string) (bool, error) { return false, nil }
func (a *adapter) GetPackages(registry string) {

}
func (a *adapter) GetVersions(registry, pkg string)       {}
func (a *adapter) GetFiles(registry, pkg, version string) {}
