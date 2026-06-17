package util

import (
	"path"
	"regexp"
	"strings"
)

const (
	puppetTarGzExt = ".tar.gz"
)

// puppetSemVerRegex validates a SemVer 2.0.0 version string. Mirrors the
// SemVerRegex used by AR's puppet handler so the migration only ever ingests
// versions the destination will accept.
var puppetSemVerRegex = regexp.MustCompile(
	`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
		`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)` +
		`(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
		`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`,
)

// puppetModuleNameRegex validates the "<author>-<module>" portion of a Puppet
// module name. Author and module each must start with a letter and contain
// alphanumerics or underscores. Hyphens are not permitted within either side.
var puppetModuleNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*-[a-zA-Z][a-zA-Z0-9_]*$`)

// ParsePuppetFileNameWithPath parses a Puppet module tarball filename of the
// form "<author>-<module>-<version>.tar.gz" and returns the module name
// ("<author>-<module>") plus version. Module names contain a single hyphen and
// versions are valid SemVer 2.0.0 strings, so the version is found by scanning
// hyphen positions left-to-right and accepting the first split where both
// sides match their respective patterns.
func ParsePuppetFileNameWithPath(filePath string) (string, string, bool) {
	fileName := path.Base(filePath)
	if !strings.HasSuffix(fileName, puppetTarGzExt) {
		return "", "", false
	}
	base := strings.TrimSuffix(fileName, puppetTarGzExt)

	for i := 0; i < len(base); i++ {
		if base[i] != '-' {
			continue
		}
		candidateName := base[:i]
		candidateVersion := base[i+1:]
		if !puppetModuleNameRegex.MatchString(candidateName) {
			continue
		}
		if !puppetSemVerRegex.MatchString(candidateVersion) {
			continue
		}
		return candidateName, candidateVersion, true
	}
	return "", "", false
}
