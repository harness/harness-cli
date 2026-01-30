package util

import (
	"fmt"
	"strings"

	"github.com/gobwas/glob"
	"github.com/harness/harness-cli/module/ar/migrate/types"
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
	case types.GENERIC, types.PYTHON, types.MAVEN, types.NUGET, types.NPM, types.DART, types.GO:
		return true
	default:
		return false
	}
}

func IsPackageLevelFilterableArtifact(artifactType types.ArtifactType) bool {

	switch artifactType {
	case types.DOCKER, types.HELM, types.HELM_LEGACY, types.RPM, types.CONDA, types.COMPOSER:
		return true
	default:
		return false
	}
}
