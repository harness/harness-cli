package migratable

import (
	"context"
	"fmt"
	"harness/module/ar/migrate/adapter"
)

type Registry struct {
	srcRegistry  string
	destRegistry string
	srcAdapter   adapter.Adapter
	destAdapter  adapter.Adapter
}

func NewRegistryJob(src adapter.Adapter, dest adapter.Adapter, srcRegistry string, destRegistry string) Job {
	return &Registry{
		srcRegistry:  srcRegistry,
		destRegistry: destRegistry,
		srcAdapter:   src,
		destAdapter:  dest,
	}
}

func (r *Registry) Info() string {
	return r.srcRegistry + ":" + r.destRegistry
}

// Pre Create registry at destination if it doesn't exist
func (r *Registry) Pre(ctx context.Context) error {
	// Send event to Harness that Registry migration Job has started.

	_, err := r.destAdapter.CreateRegistryIfDoesntExist(r.destRegistry)
	if err != nil {
		return fmt.Errorf("create registry failed: %w", err)
	}
	return nil
}

// Migrate Create down stream packages and migrate them
func (r *Registry) Migrate(ctx context.Context) error {
	return nil
}

// Post Any post processing work
func (r *Registry) Post(ctx context.Context) error {
	return nil
}
