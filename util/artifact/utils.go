package artifact

import "strings"

// ExtractUpstreamVersion extracts the upstream version from a Debian version
// For example: "1.2.3-4ubuntu1" -> "1.2.3", "2:1.5.0-1" -> "1.5.0"
func ExtractUpstreamVersion(version string) string {
	// Remove epoch if present (e.g., "2:1.5.0-1" -> "1.5.0-1")
	if idx := strings.Index(version, ":"); idx != -1 {
		version = version[idx+1:]
	}

	// Remove debian revision if present (e.g., "1.5.0-1" -> "1.5.0")
	if idx := strings.LastIndex(version, "-"); idx != -1 {
		version = version[:idx]
	}

	return version
}
