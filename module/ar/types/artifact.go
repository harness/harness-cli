package types

// Artifact represents an artifact in a registry
type Artifact struct {
	// Name is the name of the artifact (e.g., image name for container images)
	Name string

	// Tag is the version tag of the artifact
	Tag string

	// Type represents the type of artifact (e.g., "docker", "npm", "maven")
	Type string

	// Size is the size of the artifact in bytes
	Size int64

	// Digest is the content hash of the artifact
	Digest string

	// Created is when the artifact was created
	Created string

	// Metadata contains additional provider-specific information
	Metadata map[string]interface{}
}

// Repository represents a collection of artifacts
type Repository struct {
	// Name is the repository name
	Name string
	
	// Description is the repository description
	Description string
	
	// ArtifactCount is the count of artifacts in this repository
	ArtifactCount int
	
	// Provider is the registry provider (HAR, JFROG)
	Provider RegistryType
}
