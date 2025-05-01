package migrate

import (
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/types"
)

// AdapterArtifactHandler adapts between the new adapter.Adapter interface and the existing
// artifact handlers that expect SourceRegistry and DestinationRegistry
type AdapterArtifactHandler struct {
	// The original handler to delegate actual processing logic to
	handler ArtifactHandler

	// The source and destination adapters, storing them lets us convert
	// between old and new interfaces
	source      adapter.Adapter
	destination adapter.Adapter
}

// NewAdapterArtifactHandler creates a new handler that can work with our adapter.Adapter
func NewAdapterArtifactHandler(
	packageType string,
	source adapter.Adapter,
	destination adapter.Adapter,
) (*AdapterArtifactHandler, error) {
	// Create the original handler based on package type
	handler, err := NewArtifactHandler(packageType)
	if err != nil {
		return nil, err
	}

	return &AdapterArtifactHandler{
		handler:     handler,
		source:      source,
		destination: destination,
	}, nil
}

// CopyArtifact copies an artifact from source to destination using the adapter pattern
func (h *AdapterArtifactHandler) CopyArtifact(
	source SourceRegistry,
	destination DestinationRegistry,
	artifact Artifact,
	destRegistry string,
) error {
	// Instead of using the passed source and destination, we use our internal adapters
	// through the bridge implementations we created
	sourceBridge := &adapterSourceBridge{adapter: h.source}
	destBridge := &adapterDestBridge{adapter: h.destination}

	// Call the original handler with our bridge implementations
	return h.handler.CopyArtifact(sourceBridge, destBridge, artifact, destRegistry)
}

// adapterSourceBridge implements SourceRegistry by delegating to adapter.Adapter
type adapterSourceBridge struct {
	adapter adapter.Adapter
}

// ListArtifacts implements SourceRegistry.ListArtifacts
func (b *adapterSourceBridge) ListArtifacts(registry string) ([]Artifact, error) {
	// Get artifacts from the adapter
	adapterArtifacts, err := b.adapter.ListArtifacts(registry, "")
	if err != nil {
		return nil, err
	}

	// Convert to ar.Artifact
	artifacts := make([]Artifact, len(adapterArtifacts))
	for i, a := range adapterArtifacts {
		artifacts[i] = Artifact{
			Name:       a.Name,
			Version:    a.Tag,
			Type:       a.Type,
			Registry:   registry,
			Size:       a.Size,
			Properties: make(map[string]string),
		}

		// Copy metadata
		if a.Metadata != nil {
			for k, v := range a.Metadata {
				if strVal, ok := v.(string); ok {
					artifacts[i].Properties[k] = strVal
				}
			}
		}

		// Add digest to properties
		if a.Digest != "" {
			artifacts[i].Properties["digest"] = a.Digest
		}
	}

	return artifacts, nil
}

// DownloadArtifact implements SourceRegistry.DownloadArtifact
func (b *adapterSourceBridge) DownloadArtifact(artifact Artifact) ([]byte, error) {
	// Convert ar.Artifact to types.Artifact
	adapterArtifact := types.Artifact{
		Name: artifact.Name,
		Tag:  artifact.Version,
		Type: artifact.Type,
		Size: artifact.Size,
	}

	// Get the digest from properties if available
	if digest, ok := artifact.Properties["digest"]; ok {
		adapterArtifact.Digest = digest
	}

	// Call the adapter
	return b.adapter.PullArtifact(artifact.Registry, adapterArtifact)
}

// adapterDestBridge implements DestinationRegistry by delegating to adapter.Adapter
type adapterDestBridge struct {
	adapter adapter.Adapter
}

// UploadArtifact implements DestinationRegistry.UploadArtifact
func (b *adapterDestBridge) UploadArtifact(artifact Artifact, data []byte) error {
	// Convert ar.Artifact to types.Artifact
	adapterArtifact := types.Artifact{
		Name: artifact.Name,
		Tag:  artifact.Version,
		Type: artifact.Type,
		Size: artifact.Size,
	}

	// Get the digest from properties if available
	if digest, ok := artifact.Properties["digest"]; ok {
		adapterArtifact.Digest = digest
	}

	// Call the adapter
	return b.adapter.PushArtifact(artifact.Registry, adapterArtifact, data)
}

// CreateRegistry implements DestinationRegistry.CreateRegistry
func (b *adapterDestBridge) CreateRegistry(registry string) error {
	// Assume "generic" package type since we don't have access to the config here
	// In a real implementation, this could be passed in or looked up
	_, err := b.adapter.PrepareForPush(registry, "generic")
	return err
}
