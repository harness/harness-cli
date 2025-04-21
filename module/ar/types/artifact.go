package types

// Artifact represents an artifact in a registry
type Artifact struct {
	// Name is the name of the artifact (e.g., image name for container images)
	Name string

	// This is version for packages. For OCI images, it can be tag.
	Version string

	// Type represents the type of artifact
	Type ArtifactType

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
