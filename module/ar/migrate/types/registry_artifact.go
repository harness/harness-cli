package types

// RegistryArtifact represents an artifact in a registry with additional information
// This type serves as a bridge between the adapter's Artifact type and the ar module's Artifact type
type RegistryArtifact struct {
	// Name is the name of the artifact
	Name string
	
	// Version is the version tag of the artifact
	Version string
	
	// Type represents the type of artifact (e.g., "docker", "npm", "maven")
	Type string
	
	// Registry is the registry from which this artifact originates
	Registry string
	
	// Size is the size of the artifact in bytes
	Size int64
	
	// Properties contains additional metadata about the artifact
	Properties map[string]string
}
