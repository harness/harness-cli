package types

import "fmt"

// PackageSelector defines an opt-in allow-list for packages, versions, and files.
// Empty Versions means all versions of that package.
// Empty Files means all files of a selected version.
// Package, Versions, and Files are all matched case-insensitively.
type PackageSelector struct {
	Package  string   `yaml:"package"`
	Versions []string `yaml:"versions,omitempty"`
	Files    []string `yaml:"files,omitempty"`
}

// SelectorGranularity represents the level of filtering supported by an artifact type.
// Values are ordered so < comparisons work (Package < Version < File).
type SelectorGranularity int

const (
	GranularityPackage SelectorGranularity = iota
	GranularityVersion
	GranularityFile
)

// SupportedSelectorGranularity returns the maximum filtering granularity
// supported by the given artifact type.
func SupportedSelectorGranularity(t ArtifactType) SelectorGranularity {
	switch t {
	case GO:
		return GranularityVersion
	case GENERIC, RAW, MAVEN, PYTHON, NUGET, NPM, DART, PUPPET, RUBY:
		return GranularityFile
	default:
		// DOCKER, HELM, HELM_LEGACY, HELM_HTTP, RPM, DEBIAN, CONDA, COMPOSER, SWIFT, CONAN
		return GranularityPackage
	}
}

// ValidatePackageFilters validates that the provided package filters are compatible
// with the artifact type's supported granularity.
func ValidatePackageFilters(filters []PackageSelector, t ArtifactType) error {
	if len(filters) == 0 {
		return nil
	}

	g := SupportedSelectorGranularity(t)

	for i, selector := range filters {
		if selector.Package == "" {
			return fmt.Errorf("packageFilters[%d]: package name must not be empty", i)
		}

		if len(selector.Versions) > 0 && g < GranularityVersion {
			return fmt.Errorf("packageFilters[%d]: artifact type %s does not support version-level filtering", i, t)
		}

		if len(selector.Files) > 0 && g < GranularityFile {
			return fmt.Errorf("packageFilters[%d]: artifact type %s does not support file-level filtering", i, t)
		}
	}

	return nil
}
