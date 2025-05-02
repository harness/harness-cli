package types

type InputMapping struct {
	SourceRegistry       string
	DestinationRegistry  string
	ArtifactType         ArtifactType
	ArtifactNamePatterns struct {
		Include []string
		Exclude []string
	}
}
