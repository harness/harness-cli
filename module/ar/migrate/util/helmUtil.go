package util

import (
	"path"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// helmChartExt is the canonical Helm chart archive extension.
const helmChartExt = ".tgz"

// helmProvExt is the provenance sidecar suffix appended to a chart archive name.
const helmProvExt = ".prov"

// ParseChartFileName splits a Helm chart file name like "{name}-{version}.tgz"
// (or "{name}-{version}.tgz.prov") into its name and version components.
//
// This is a client-side port of HAR's server-side parser
// (artifact-registry: registry/pkg/helmhttp/helper.go ParseChartFileName) and
// MUST stay behaviourally identical, since the server re-parses the uploaded
// file name and rejects mismatches. The boundary between name and version is
// found by trying each hyphen left-to-right and accepting the first split whose
// right-hand side is a valid SemVer 2 string. This correctly handles both
// hyphenated chart names (e.g. "prometheus-mysql-exporter") and hyphenated
// prerelease versions (e.g. "1.21.0-alpha.1"), which a naive last-hyphen split
// would mis-parse.
//
// Only the leaf segment is parsed: any directory prefix (e.g.
// "ChartA/ChartB/abc-1.0.1.tgz") is stripped first via path.Base, mirroring the
// server which derives the chart name from path.Base(filePath). ok is false
// when the name does not conform to the "<name>-<semver>" convention.
func ParseChartFileName(filename string) (name, version string, ok bool) {
	// Reduce to the leaf segment; directory prefixes are not part of the
	// name<->version split (parity with server-side path.Base handling).
	base := path.Base(filename)

	// Strip the optional .prov provenance suffix first, then the .tgz suffix.
	base = strings.TrimSuffix(base, helmProvExt)
	base = strings.TrimSuffix(base, helmChartExt)

	for i := 0; i < len(base); i++ {
		if base[i] != '-' {
			continue
		}
		candidateName := base[:i]
		candidateVersion := base[i+1:]
		if candidateName == "" || candidateVersion == "" {
			continue
		}
		if _, err := semver.NewVersion(candidateVersion); err == nil {
			return candidateName, candidateVersion, true
		}
	}

	return "", "", false
}

// GetChartFileName returns the canonical chart archive file name for a given
// chart name and version: "<name>-<version>.tgz".
//
// name may carry a nested directory prefix (e.g. "ChartA/ChartB/abc") produced
// by the JFrog adapter's getNestedName; the prefix is preserved verbatim so the
// upload path mirrors the source layout. The "<leaf>-<version>.tgz" form is
// what the server re-parses (it strips the prefix and validates the leaf
// against Chart.yaml).
func GetChartFileName(name, version string) string {
	return name + "-" + version + helmChartExt
}

// GetChartProvFileName returns the provenance sidecar file name for a chart:
// "<name>-<version>.tgz.prov".
func GetChartProvFileName(name, version string) string {
	return GetChartFileName(name, version) + helmProvExt
}

// IsHelmChartArchive reports whether a file name is a Helm chart archive
// (".tgz") and not a provenance sidecar (".tgz.prov"). Used by the tree-sweep
// enumeration to select chart files while excluding their .prov siblings,
// checksum files, and the index.yaml itself.
func IsHelmChartArchive(name string) bool {
	return strings.HasSuffix(name, helmChartExt) && !strings.HasSuffix(name, helmChartExt+helmProvExt)
}
