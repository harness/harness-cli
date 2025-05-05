package types

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
