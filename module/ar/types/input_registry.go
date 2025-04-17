package types

type InputRegistry struct {
	SourceRegistry       string
	DestinationRegistry  string
	ArtifactNamePatterns struct {
		Include []string
		Exclude []string
	}
}
