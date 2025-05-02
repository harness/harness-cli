package jfrog

import (
	"context"
	adp "harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
)

func init() {
	adapterType := types.HAR
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

// factory section
type factory struct {
}

type adapter struct {
	client *client
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	c := newClient(&config)
	return &adapter{
		client: c,
	}, nil
}

func (a *adapter) ValidateCredentials() (bool, error)               { return false, nil }
func (a *adapter) GetRegistry(registry string) (interface{}, error) { return nil, nil }
func (a *adapter) CreateRegistryIfDoesntExist(registryRef string) (bool, error) {
	return false, nil
}
func (a *adapter) GetPackages(registry string)            {}
func (a *adapter) GetVersions(registry, pkg string)       {}
func (a *adapter) GetFiles(registry, pkg, version string) {}
