package registry

import (
	"errors"
	"fmt"
	"harness/internal/config"
)

// Common errors
var (
	ErrUnsupportedRegistryType = errors.New("unsupported registry type")
	ErrArtifactNotFound        = errors.New("artifact not found")
	ErrRegistryNotFound        = errors.New("registry not found")
	ErrInvalidCredentials      = errors.New("invalid credentials")
)

// Artifact represents a single artifact in a registry
type Artifact struct {
	Name       string
	Version    string
	Type       string
	Registry   string
	Size       int64
	Properties map[string]string
}

// SourceRegistry defines the interface for source registries
type SourceRegistry interface {
	// ListArtifacts lists all artifacts in a registry
	ListArtifacts(registry string) ([]Artifact, error)

	// DownloadArtifact downloads an artifact from the registry
	DownloadArtifact(artifact Artifact) ([]byte, error)
}

// DestinationRegistry defines the interface for destination registries
type DestinationRegistry interface {
	// UploadArtifact uploads an artifact to the registry
	UploadArtifact(artifact Artifact, data []byte) error

	// CreateRegistry creates a new registry
	CreateRegistry(registry string) error
}

// NewSourceRegistry creates a new source registry client based on the configuration
func NewSourceRegistry(cfg config.SourceConfig) (SourceRegistry, error) {
	switch cfg.Type {
	case "JFROG":
		return NewJFrogSourceRegistry(cfg)
	// Add more source registry types here
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedRegistryType, cfg.Type)
	}
}

// NewDestinationRegistry creates a new destination registry client based on the configuration
func NewDestinationRegistry(cfg config.DestinationConfig) (DestinationRegistry, error) {
	switch cfg.Type {
	case "HAR":
		return NewHARDestinationRegistry(cfg)
	// Add more destination registry types here
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedRegistryType, cfg.Type)
	}
}
