package util

import (
	"errors"
	"path"
	"regexp"
	"strings"
)

const rubyGemExt = ".gem"

var errInvalidGemFilename = errors.New("invalid gem filename")

// rubyGemVersionPattern matches RubyGems dot-prerelease versions in filenames
// (e.g. 1.0.0.beta1). Mirrors artifact-registry registry/pkg/ruby ParseGemFilename.
var rubyGemVersionPattern = regexp.MustCompile(`^\d+\.\d+(\.\d+)?(\.[0-9A-Za-z\.]+)*$`)

// RubyMetadata holds parsed fields from a RubyGems filename
// ({name}-{version}.gem or {name}-{version}-{platform}.gem).
type RubyMetadata struct {
	Name     string
	Version  string
	Platform string
}

// ParseRubyGemFileNameWithPath parses a RubyGems download filename from a
// registry-relative path and returns the gem metadata. Multiple platform
// variants for the same version are grouped under one version entry during
// migration; Platform is populated when present in the filename.
func ParseRubyGemFileNameWithPath(filePath string) (RubyMetadata, bool) {
	meta, err := parseGemFilename(path.Base(filePath))
	if err != nil {
		return RubyMetadata{}, false
	}
	return meta, true
}

func parseGemFilename(filename string) (RubyMetadata, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return RubyMetadata{}, errInvalidGemFilename
	}
	stem, ok := strings.CutSuffix(filename, rubyGemExt)
	if !ok {
		return RubyMetadata{}, errInvalidGemFilename
	}

	parts := strings.Split(stem, "-")
	if len(parts) < 2 {
		return RubyMetadata{}, errInvalidGemFilename
	}

	for i := len(parts) - 1; i >= 1; i-- {
		candidateVersion := parts[i]
		if !rubyGemVersionPattern.MatchString(candidateVersion) {
			continue
		}
		name := strings.Join(parts[:i], "-")
		if name == "" {
			continue
		}
		meta := RubyMetadata{
			Name:    name,
			Version: candidateVersion,
		}
		if i+1 < len(parts) {
			meta.Platform = strings.Join(parts[i+1:], "-")
		}
		return meta, nil
	}
	return RubyMetadata{}, errInvalidGemFilename
}
