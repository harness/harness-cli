package gopkg

// Origin represents the VCS metadata of a Go package
type Origin struct {
	VCS  string `json:"VCS,omitempty"`
	URL  string `json:"URL,omitempty"`
	Ref  string `json:"Ref,omitempty"`
	Hash string `json:"Hash,omitempty"`
}

// PackageMetadata represents the metadata of a Go package
type PackageMetadata struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
	Origin  Origin `json:"Origin,omitempty"`
}

// zipEntry represents a file to be added to the zip archive
type zipEntry struct {
	sourcePath string
	zipPath    string
}
