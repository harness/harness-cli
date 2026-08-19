package types

// Pattern-filterability classification for includePatterns/excludePatterns.
//
// This lives in the types package (not migrate/util) so config validation can
// reject scope controls that would be silently ignored, without an import
// cycle. migrate/util's IsFileLevelFilterableArtifact /
// IsPackageLevelFilterableArtifact delegate here — keep the classification in
// sync with the actual filter application sites in migratable/registry.go.

// IsFileLevelPatternFilterable reports whether include/exclude patterns are
// applied to individual file URIs for this artifact type.
func IsFileLevelPatternFilterable(t ArtifactType) bool {
	switch t {
	case GENERIC, RAW, PYTHON, MAVEN, NUGET, NPM, DART, GO, RUBY, PUPPET, TERRAFORM:
		return true
	default:
		return false
	}
}

// IsPackageLevelPatternFilterable reports whether include/exclude patterns are
// applied to package names for this artifact type.
func IsPackageLevelPatternFilterable(t ArtifactType) bool {
	switch t {
	case DOCKER, HELM, HELM_LEGACY, HELM_HTTP, RPM, CONDA, COMPOSER, SWIFT, CONAN, DEBIAN:
		return true
	default:
		return false
	}
}

// IsPatternFilterable reports whether include/exclude patterns have ANY effect
// for this artifact type (at either file or package level).
func IsPatternFilterable(t ArtifactType) bool {
	return IsFileLevelPatternFilterable(t) || IsPackageLevelPatternFilterable(t)
}
