package util

import (
	"path"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// composerSemverSuffix matches a semver (or four-part) suffix at the end of a zip base name.
var composerSemverSuffix = regexp.MustCompile(`-(\d+\.\d+\.\d+(?:\.\d+)?)$`)

// ComposerLogicalNameFromZipURI derives the logical Composer package name (vendor/package)
// from a registry-relative .zip URI. This matches HAR/Composer identity (see
// artifact-registry registry/pkg/composer ParsePackageName).
//
// Migration configs must use the same vendor/package names in includePatterns,
// excludePatterns, and packageFilters (not legacy zip basenames).
//
// Supported layouts:
//   - /vendor/package/any.zip              -> vendor/package
//   - /vendor-package/file.zip             -> vendor/package (directory split on first '-')
//   - /vendor-package-version.zip          -> vendor/package (repo root; version stripped from basename)
//
// Flat layout assumes a single '-' separates vendor from package. Vendors whose
// names contain hyphens (e.g. my-org/widget) must use the /vendor/package/ path
// layout; /my-org-widget/file.zip would be parsed as my/org-widget.
func ComposerLogicalNameFromZipURI(uri string) (string, bool) {
	uri = strings.TrimPrefix(uri, "/")
	if uri == "" || !strings.HasSuffix(uri, ".zip") {
		return "", false
	}
	parts := strings.Split(uri, "/")
	switch len(parts) {
	case 1:
		base := strings.TrimSuffix(parts[0], ".zip")
		m := composerSemverSuffix.FindStringSubmatch(base)
		if len(m) != 2 {
			return "", false
		}
		name := strings.TrimSuffix(base, m[0])
		name = strings.TrimSuffix(name, "-")
		if name == "" {
			return "", false
		}
		return composerLogicalNameFromSegment(name, uri)
	case 2:
		return composerLogicalNameFromSegment(parts[0], uri)
	default:
		return parts[0] + "/" + parts[1], true
	}
}

func composerLogicalNameFromSegment(segment, uri string) (string, bool) {
	if i := strings.Index(segment, "-"); i > 0 {
		logical := segment[:i] + "/" + segment[i+1:]
		if strings.Contains(segment[i+1:], "-") {
			log.Debug().Msgf(
				"Composer zip %q: name %q has multiple '-' segments; parsed as %q (use /vendor/package/ layout if vendor name contains hyphens)",
				uri, segment, logical,
			)
		}
		return logical, true
	}
	return segment, true
}

// ComposerVersionFromZipFilename extracts the semver version from a Composer zip file name.
func ComposerVersionFromZipFilename(filename string) string {
	base := strings.TrimSuffix(filename, ".zip")
	if m := composerSemverSuffix.FindStringSubmatch(base); len(m) == 2 {
		return m[1]
	}
	// Fallback: entire base name (e.g. 3.0.0.0.zip).
	return base
}

// ComposerZipBasename returns the file name portion of a registry-relative URI.
func ComposerZipBasename(uri string) string {
	return path.Base(strings.TrimPrefix(uri, "/"))
}
