package util

import (
	"path"
	"regexp"
	"strings"
)

const (
	composerZipExt = ".zip"
)

// composerSemVerRegex validates a SemVer-ish version string used by Composer.
// Composer versions follow SemVer 2.0.0 but also allow a "v" prefix and
// Composer-style stability flags (e.g. 1.0.0-alpha1, 2.0.0-patch.1).
var composerSemVerRegex = regexp.MustCompile(
	`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
		`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)` +
		`(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
		`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`,
)

// composerPkgNameRegex validates the "vendor-package" portion of a Composer
// archive name. Both vendor and package may contain alphanumerics, hyphens,
// and underscores; they are separated by a single hyphen.
var composerPkgNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+-[a-zA-Z0-9_-]+$`)

// ParseComposerFileName parses a Composer package archive filename of the form
// "<vendor>-<package>-<version>.zip" (with an optional leading path).
// It returns the logical package name ("<vendor>-<package>"), the version
// string, and true on success; empty strings and false if the filename does
// not match the expected pattern.
//
// The split strategy mirrors ParsePuppetFileNameWithPath: scan hyphen positions
// left-to-right and accept the first split where both sides match their
// respective patterns.
func ParseComposerFileName(filePath string) (string, string, bool) {
	fileName := path.Base(filePath)
	if !strings.HasSuffix(fileName, composerZipExt) {
		return "", "", false
	}

	base := strings.TrimSuffix(fileName, composerZipExt)

	for i := 0; i < len(base)-1; i++ {
		if base[i] != '-' {
			continue
		}
		candidateName := base[:i]
		candidateVersion := base[i+1:]
		if !composerPkgNameRegex.MatchString(candidateName) {
			continue
		}
		if !composerSemVerRegex.MatchString(candidateVersion) {
			continue
		}
		return candidateName, candidateVersion, true
	}

	return "", "", false
}
