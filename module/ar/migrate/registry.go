package migrate

import (
	"fmt"
	"harness/module/ar/migrate/types"
)

// InitializeRegistryFactories creates the registry factories for all supported registry types
func InitializeRegistryFactories() {
	// This function could do additional initialization work if needed
	// Currently, registry factories are automatically registered via init() functions
}

// CreateSourceRegistry creates a new source registry based on the config type
func CreateSourceRegistry(cfg types.RegistryConfig) (SourceRegistry, error) {
	switch cfg.Type {
	case types.HAR, types.JFROG:
		return NewAdapterSourceRegistry(cfg)
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", cfg.Type)
	}
}

// CreateDestinationRegistry creates a new destination registry based on the config type
func CreateDestinationRegistry(cfg types.RegistryConfig) (DestinationRegistry, error) {
	switch cfg.Type {
	case types.HAR, types.JFROG:
		return NewAdapterDestinationRegistry(cfg)
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", cfg.Type)
	}
}
