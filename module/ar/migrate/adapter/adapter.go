package adapter

import (
	"context"
	"errors"
	"fmt"
	"harness/module/ar/migrate/types"
)

type Adapter interface {
	// ListArtifacts lists all artifacts from a specified registry
	ListArtifacts(registry string, artifactType types.ArtifactType) ([]types.Artifact, error)

	// CreateRegistry creates a registry in the system
	PrepareForPush(registry string, packageType string) (string, error)

	// PullArtifact pulls an artifact from the source registry
	PullArtifact(registry string, artifact types.Artifact) ([]byte, error)

	// PushArtifact pushes an artifact to the destination registry
	PushArtifact(registry string, artifact types.Artifact, data []byte) error
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
