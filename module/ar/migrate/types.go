package migrate

import (
	"errors"
)

// Common errors
var (
	ErrUnsupportedRegistryType = errors.New("unsupported ar type")
	ErrArtifactNotFound        = errors.New("artifact not found")
	ErrRegistryNotFound        = errors.New("ar not found")
	ErrInvalidCredentials      = errors.New("invalid credentials")
)

// Artifact represents a single artifact in ar
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
	// ListArtifacts lists all artifacts in ar
	ListArtifacts(registry string) ([]Artifact, error)

	// DownloadArtifact downloads an artifact from the ar
	DownloadArtifact(artifact Artifact) ([]byte, error)
}

// DestinationRegistry defines the interface for destination registries
type DestinationRegistry interface {
	// UploadArtifact uploads an artifact to the ar
	UploadArtifact(artifact Artifact, data []byte) error

	// CreateRegistry creates a new ar
	CreateRegistry(registry string) error
}

// Legacy function types for reference (implementation moved to adapter_bridge.go)
//
// func NewSourceRegistry(cfg types.RegistryConfig) (SourceRegistry, error)
// func NewDestinationRegistry(cfg types.RegistryConfig) (DestinationRegistry, error)
