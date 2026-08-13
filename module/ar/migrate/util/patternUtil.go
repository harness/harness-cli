package util

import (
	"fmt"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/gobwas/glob"
)

/* Patterns support * and ** wildcards:
- * matches files in the current directory only (single level)
- ** matches files in all subdirectories recursively (multi-level)
*/

func MatchesPattern(filePath string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}

	// removing leading slashes
	normalizedPath := strings.TrimPrefix(filePath, "/")

	for _, pattern := range patterns {
		normalizedPattern := strings.TrimPrefix(pattern, "/")

		// Validate pattern - only * and ** wildcards are supported
		if containsUnsupportedWildcards(normalizedPattern) {
			fmt.Printf("WARNING: Pattern '%s' contains unsupported wildcard characters. Only * and ** are supported.\n", pattern)
			continue
		}

		// The library handles *  and **  natively
		g, err := glob.Compile(normalizedPattern, '/')
		if err != nil {
			// If pattern compilation fails, skip this pattern
			continue
		}

		// Check if the path matches the pattern
		if g.Match(normalizedPath) {
			return true
		}
	}

	return false
}

// containsUnsupportedWildcards checks if pattern contains unsupported wildcard characters
// Only * and ** are supported. Characters like ?, [, ], {, } are not supported.
func containsUnsupportedWildcards(pattern string) bool {
	unsupportedChars := []rune{'?', '[', ']', '{', '}'}

	for _, char := range unsupportedChars {
		if strings.ContainsRune(pattern, char) {
			return true
		}
	}

	return false
}

func IsWildCardExpression(pattern string) (bool, error) {
	unsupportedChars := []rune{'[', ']', '{', '}'}
	supportedChars := []rune{'?', '*'}

	for _, char := range unsupportedChars {
		if strings.ContainsRune(pattern, char) {
			return false, fmt.Errorf(
				"unsupported wildcard character %q found in pattern %q; only '*' and '?' are supported",
				char,
				pattern,
			)
		}
	}

	for _, char := range supportedChars {
		if strings.ContainsRune(pattern, char) {
			return true, nil
		}
	}

	return false, nil
}

// FilterFilesByPatterns filters a list of files based on include and exclude patterns.
// Include patterns are applied first (if any), then exclude patterns are applied.
// If no include patterns are specified, all files are included by default.

func FilterFilesByPatterns(files []types.File, includePatterns, excludePatterns []string) []types.File {
	if len(includePatterns) == 0 && len(excludePatterns) == 0 {
		return files
	}

	var filtered []types.File
	for _, file := range files {
		// Skipping folders
		if file.Folder {
			filtered = append(filtered, file)
			continue
		}

		if len(includePatterns) > 0 {
			if !MatchesPattern(file.Uri, includePatterns) {
				// File doesn't match any include pattern, skip it
				continue
			}
		} else if len(excludePatterns) > 0 {
			if MatchesPattern(file.Uri, excludePatterns) {
				// File matches an exclude pattern, skip it
				continue
			}
		}
		//appending passed files
		filtered = append(filtered, file)
	}

	return filtered
}

// This is to filter based on package name
func FilterFilesByPatternsPackageName(packages []types.Package, includePatterns, excludePatterns []string) []types.Package {
	if len(includePatterns) == 0 && len(excludePatterns) == 0 {
		return packages
	}

	var filteredPackages []types.Package
	for _, pkg := range packages {

		if len(includePatterns) > 0 {
			if !MatchesPattern(pkg.Name, includePatterns) {
				continue
			}
		} else if len(excludePatterns) > 0 {
			if MatchesPattern(pkg.Name, excludePatterns) {
				continue
			}
		}
		filteredPackages = append(filteredPackages, pkg)
	}

	return filteredPackages
}

func IsFileLevelFilterableArtifact(artifactType types.ArtifactType) bool {
	switch artifactType {
	case types.GENERIC, types.RAW, types.PYTHON, types.MAVEN, types.NUGET, types.NPM, types.DART, types.GO:
		return true
	default:
		return false
	}
}

func IsPackageLevelFilterableArtifact(artifactType types.ArtifactType) bool {

	switch artifactType {
	case types.DOCKER, types.HELM, types.HELM_LEGACY, types.HELM_HTTP, types.RPM, types.CONDA, types.COMPOSER, types.SWIFT, types.CONAN:
		return true
	default:
		return false
	}
}

// IsAtomicVersionArtifact reports whether a single logical version of this
// artifact type may span MULTIPLE distribution files that must be migrated
// all-or-nothing. For such types, GetVersions returns several types.Version
// entries sharing the same Name (one per distribution) — e.g. a PyPI release
// is an sdist plus one or more wheels. Because the date/pattern filters run
// per file, they can keep some distributions of a version and prune others;
// migrating only the survivors would publish a PARTIAL version. Package.Migrate
// therefore groups these entries by version.Name and migrates a version when
// ANY of its files survived the filter, recovering the pruned distributions
// from the unfiltered tree so partial versions are never published.
//
// Types NOT listed here have exactly one file per version entry (or a unique
// Name per version), so grouping is a no-op for them and Package.Migrate keeps
// the per-entry path. To onboard another multi-file-version ecosystem, add it
// here AFTER confirming its GetVersions emits one entry per distribution file
// (sharing Name) rather than one entry per version.
func IsAtomicVersionArtifact(artifactType types.ArtifactType) bool {
	switch artifactType {
	case types.PYTHON:
		return true
	default:
		return false
	}
}

func IsTimeBasedFilterPresent(mapping *types.RegistryMapping) bool {
	if mapping.DateFilter != nil {
		return true
	}
	return false

}

// IsMetadataDrivenArtifact returns true for artifact types whose GetPackages
// implementation reads a repository metadata file (e.g. RPM's primary.xml.gz,
// Debian's Packages.gz) that enumerates ALL packages in the repo, regardless
// of which files are present in the date-filtered tree.
// For these types the date filter must be re-applied after GetPackages.
func IsMetadataDrivenArtifact(artifactType types.ArtifactType) bool {
	switch artifactType {
	case types.RPM, types.DEBIAN:
		return true
	default:
		return false
	}
}

// support * and ?
func MatchesWildCardPattern(packageName string, pattern string) bool {
	g, err := glob.Compile(pattern)
	if err != nil {
		return false
	}
	return g.Match(packageName)
}
