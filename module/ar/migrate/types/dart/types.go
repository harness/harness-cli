package dart

// Pubspec represents the pubspec.yaml file structure in a Dart package
type Pubspec struct {
	Name            string                 `yaml:"name" json:"name"`
	Version         string                 `yaml:"version" json:"version"`
	Description     string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Homepage        string                 `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	Repository      string                 `yaml:"repository,omitempty" json:"repository,omitempty"`
	Environment     map[string]string      `yaml:"environment,omitempty" json:"environment,omitempty"`
	Dependencies    map[string]interface{} `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	DevDependencies map[string]interface{} `yaml:"dev_dependencies,omitempty" json:"dev_dependencies,omitempty"`
	Authors         []string               `yaml:"authors,omitempty" json:"authors,omitempty"`
}

// PackageUpload represents the upload payload for a Dart package
type PackageUpload struct {
	Name    string  `json:"name"`
	Version string  `json:"version"`
	Pubspec Pubspec `json:"pubspec"`
}
