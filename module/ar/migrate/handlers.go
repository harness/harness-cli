package migrate

import (
	"fmt"
	"log"
)

// ArtifactHandler defines the interface for package-type-specific artifact handlers
type ArtifactHandler interface {
	// CopyArtifact copies an artifact from source to destination
	CopyArtifact(source SourceRegistry, destination DestinationRegistry, artifact Artifact, destRegistry string) error
}

// NewArtifactHandler creates an appropriate artifact handler based on the package type
func NewArtifactHandler(packageType string) (ArtifactHandler, error) {
	switch packageType {
	case "generic":
		return &GenericArtifactHandler{}, nil
	case "python":
		return &PythonArtifactHandler{}, nil
	case "maven":
		return &MavenArtifactHandler{}, nil
	case "npm":
		return &NpmArtifactHandler{}, nil
	default:
		// Fallback to generic handler if no specific handler is found
		log.Printf("No specific handler found for package type: %s, using generic handler", packageType)
		return &GenericArtifactHandler{}, nil
	}
}

// GenericArtifactHandler implements the standard artifact copying logic
type GenericArtifactHandler struct{}

// CopyArtifact for generic artifacts performs a simple download and upload
func (h *GenericArtifactHandler) CopyArtifact(
	source SourceRegistry,
	destination DestinationRegistry,
	artifact Artifact,
	destRegistry string,
) error {
	// Download artifact
	log.Printf("Downloading generic artifact %s:%s", artifact.Name, artifact.Version)
	artifactBytes, err := source.DownloadArtifact(artifact)
	if err != nil {
		return fmt.Errorf("failed to download generic artifact: %w", err)
	}

	// Upload artifact
	log.Printf("Uploading generic artifact %s:%s to destination", artifact.Name, artifact.Version)
	destArtifact := Artifact{
		Name:       artifact.Name,
		Version:    artifact.Version,
		Type:       artifact.Type,
		Registry:   destRegistry,
		Properties: artifact.Properties,
	}

	if err := destination.UploadArtifact(destArtifact, artifactBytes); err != nil {
		return fmt.Errorf("failed to upload generic artifact: %w", err)
	}

	return nil
}

// PythonArtifactHandler implements Python-specific artifact copying logic
type PythonArtifactHandler struct{}

// CopyArtifact for Python packages handles Python-specific versioning and metadata
func (h *PythonArtifactHandler) CopyArtifact(
	source SourceRegistry,
	destination DestinationRegistry,
	artifact Artifact,
	destRegistry string,
) error {
	// Download artifact
	log.Printf("Downloading Python package %s:%s", artifact.Name, artifact.Version)
	artifactBytes, err := source.DownloadArtifact(artifact)
	if err != nil {
		return fmt.Errorf("failed to download Python package: %w", err)
	}

	// For Python, we might need to handle wheel vs. sdist differently
	// or process requirements.txt or pyproject.toml information
	packageFormat := "unknown"
	if prop, ok := artifact.Properties["format"]; ok {
		packageFormat = prop
	}

	log.Printf("Processing Python package in %s format", packageFormat)

	// We could extract metadata or validate the package here
	// For demo purposes, we're just logging the format

	// Upload artifact
	log.Printf("Uploading Python package %s:%s to destination", artifact.Name, artifact.Version)
	destArtifact := Artifact{
		Name:       artifact.Name,
		Version:    artifact.Version,
		Type:       artifact.Type,
		Registry:   destRegistry,
		Properties: artifact.Properties,
	}

	// Python packages should maintain the original filename
	// We could ensure that here by setting a specific property

	if err := destination.UploadArtifact(destArtifact, artifactBytes); err != nil {
		return fmt.Errorf("failed to upload Python package: %w", err)
	}

	return nil
}

// MavenArtifactHandler implements Maven-specific artifact copying logic
type MavenArtifactHandler struct{}

// CopyArtifact for Maven artifacts handles Maven coordinates and POM files
func (h *MavenArtifactHandler) CopyArtifact(
	source SourceRegistry,
	destination DestinationRegistry,
	artifact Artifact,
	destRegistry string,
) error {
	// Download artifact
	log.Printf("Downloading Maven artifact %s:%s", artifact.Name, artifact.Version)
	artifactBytes, err := source.DownloadArtifact(artifact)
	if err != nil {
		return fmt.Errorf("failed to download Maven artifact: %w", err)
	}

	// For Maven, we would handle groupId:artifactId:version coordinates
	// and potentially process POM files

	// We might need to copy related artifacts like POM files, sources, etc.
	// This is a simplified implementation for demo purposes

	// Upload artifact
	log.Printf("Uploading Maven artifact %s:%s to destination", artifact.Name, artifact.Version)
	destArtifact := Artifact{
		Name:       artifact.Name,
		Version:    artifact.Version,
		Type:       artifact.Type,
		Registry:   destRegistry,
		Properties: artifact.Properties,
	}

	if err := destination.UploadArtifact(destArtifact, artifactBytes); err != nil {
		return fmt.Errorf("failed to upload Maven artifact: %w", err)
	}

	return nil
}

// NpmArtifactHandler implements NPM-specific artifact copying logic
type NpmArtifactHandler struct{}

// CopyArtifact for NPM packages handles package.json and scoped packages
func (h *NpmArtifactHandler) CopyArtifact(
	source SourceRegistry,
	destination DestinationRegistry,
	artifact Artifact,
	destRegistry string,
) error {
	// Download artifact
	log.Printf("Downloading NPM package %s:%s", artifact.Name, artifact.Version)
	artifactBytes, err := source.DownloadArtifact(artifact)
	if err != nil {
		return fmt.Errorf("failed to download NPM package: %w", err)
	}

	// For NPM, we might need to handle scoped packages (@org/package-name)
	// and versioning like major.minor.patch-tag

	// We might also need to preserve or update package.json contents

	// Upload artifact
	log.Printf("Uploading NPM package %s:%s to destination", artifact.Name, artifact.Version)
	destArtifact := Artifact{
		Name:       artifact.Name,
		Version:    artifact.Version,
		Type:       artifact.Type,
		Registry:   destRegistry,
		Properties: artifact.Properties,
	}

	if err := destination.UploadArtifact(destArtifact, artifactBytes); err != nil {
		return fmt.Errorf("failed to upload NPM package: %w", err)
	}

	return nil
}
