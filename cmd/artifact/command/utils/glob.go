package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ResolveFilePath resolves a file path pattern (supports glob wildcards and regex).
// If the pattern contains wildcards/regex, it returns all matching files.
// If no wildcards, it returns the path as-is after validating it exists.
// Optional extensions filter can be provided to only include files with specific extensions.
func ResolveFilePath(pattern string, extensions ...string) ([]string, error) {
	// Check if pattern contains glob/regex characters
	if !containsPattern(pattern) {
		// No pattern, validate file exists
		if _, err := os.Stat(pattern); err != nil {
			return nil, fmt.Errorf("failed to access file: %w", err)
		}
		return []string{pattern}, nil
	}

	// Try glob first (simpler patterns like *.tgz)
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		return filterByExtensions(matches, pattern, extensions)
	}

	// Fall back to regex matching in current directory
	dir := filepath.Dir(pattern)
	if dir == "" || dir == "." {
		dir = "."
	}
	regexPattern := filepath.Base(pattern)

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var regexMatches []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if re.MatchString(filepath.Base(path)) {
			regexMatches = append(regexMatches, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(regexMatches) == 0 {
		return nil, fmt.Errorf("no files matched pattern: %s", pattern)
	}

	return filterByExtensions(regexMatches, pattern, extensions)
}

func filterByExtensions(matches []string, pattern string, extensions []string) ([]string, error) {
	if len(extensions) == 0 {
		return matches, nil
	}

	extMap := make(map[string]bool)
	for _, ext := range extensions {
		extMap[ext] = true
	}

	var filtered []string
	for _, match := range matches {
		if extMap[filepath.Ext(match)] {
			filtered = append(filtered, match)
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no files with extensions %v matched pattern: %s", extensions, pattern)
	}
	return filtered, nil
}

func containsPattern(pattern string) bool {
	for _, c := range pattern {
		if c == '*' || c == '?' || c == '[' || c == '(' || c == '|' || c == '+' || c == '\\' {
			return true
		}
	}
	return false
}
