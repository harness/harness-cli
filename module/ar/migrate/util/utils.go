package util

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

func GenOCIImagePath(host string, pathParams ...string) string {
	params := strings.Join(pathParams, "/")
	return fmt.Sprintf("%s/%s", host, params)
}

func GetRegistryRef(account string, ref string, registry string) string {
	result := []string{account}
	ref = strings.TrimSuffix(ref, "/")
	ref = strings.TrimPrefix(ref, "/")
	registry = strings.TrimPrefix(registry, "/")
	registry = strings.TrimSuffix(registry, "/")
	if ref != "" {
		result = append(result, ref)
	}
	result = append(result, registry)
	return strings.Join(result, "/")
}

func GetSkipPrinter() *pterm.PrefixPrinter {
	return &pterm.PrefixPrinter{
		MessageStyle: &pterm.ThemeDefault.WarningMessageStyle,
		Prefix: pterm.Prefix{
			Style: &pterm.ThemeDefault.WarningPrefixStyle,
			Text:  "SKIPPED",
		},
		Writer: os.Stdout,
	}
}

func parseDate(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %q", s)
}

func buildURI(path, name string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	if path == "" || path == "." {
		return "/" + name
	}
	return "/" + path + "/" + name
}

func onOrAfter(t, threshold time.Time) bool {
	return !t.Before(threshold)
}

func FilterFilesByDate(files []types.File, filteredURIs map[string]struct{}) []types.File {
	var result []types.File
	for _, f := range files {
		if _, ok := filteredURIs[f.Uri]; ok {
			result = append(result, f)
		}
	}
	return result
}

// FilterPackagesByFileName filters packages by checking if each package's exists among the
// date-filtered files. This is used for metadata-driven artifact types (RPM,
// DEBIAN) where GetPackages reads a metadata file that lists every package in
// the repository, ignoring the date-filtered tree.

func FilterPackagesByFileName(pkgs []types.Package, dateFilteredFiles []types.File) []types.Package {
	uriSet := make(map[string]struct{}, len(dateFilteredFiles))
	for _, f := range dateFilteredFiles {
		uriSet[strings.TrimPrefix(f.Uri, "/")] = struct{}{}
	}

	var result []types.Package
	for _, pkg := range pkgs {
		if _, ok := uriSet[pkg.URI]; ok {
			result = append(result, pkg)
		}
	}
	return result
}

func CreateMapOfFilteredFile(searchedFiles []types.SearchedFile, mapping *types.RegistryMapping) map[string]struct{} {
	result := map[string]struct{}{}

	if mapping.DateFilter == nil {
		return result
	}

	df := mapping.DateFilter
	hasCreated := df.CreatedAfter != nil
	hasDownloaded := df.DownloadedAfter != nil

	log.Info().Msgf("Filtering files by dateFilter (match: %s, createdAfter: %v, downloadedAfter: %v)",
		df.Match, df.CreatedAfter, df.DownloadedAfter)

	for _, f := range searchedFiles {
		var matchedCreated, matchedDownloaded bool

		if hasCreated {
			created, err := parseDate(f.Created)
			if err != nil {
				log.Warn().Msgf("File %s: failed to parse created date %q: %v", f.Name, f.Created, err)
			} else {
				matchedCreated = onOrAfter(created, *df.CreatedAfter)
			}
		}

		if hasDownloaded {
			for _, stat := range f.Stats {
				downloaded, err := parseDate(stat.Downloaded)
				if err != nil {
					log.Warn().Msgf("File %s: failed to parse downloaded date %q: %v", f.Name, stat.Downloaded, err)
					continue
				}
				if onOrAfter(downloaded, *df.DownloadedAfter) {
					matchedDownloaded = true
					break
				}
			}
		}

		var include bool
		switch df.Match {
		case types.DateFilterMatchAny:
			include = (hasCreated && matchedCreated) || (hasDownloaded && matchedDownloaded)
		case types.DateFilterMatchAll:
			include = true
			if hasCreated && !matchedCreated {
				include = false
			}
			if hasDownloaded && !matchedDownloaded {
				include = false
			}
		}

		if include {
			//if all condition met add URI in map
			result[buildURI(f.Path, f.Name)] = struct{}{}
		}
	}

	return result
}

// IsPackageIndexFile reports whether uri is a repository index/metadata file
// that package enumeration depends on (rather than an actual artifact). Such
// files must be exempt from the date filter: they are typically old (written
// once, rarely re-downloaded) and would otherwise be dropped by a
// createdAfter/downloadedAfter filter, breaking enumeration for the whole
// registry even though the artifacts themselves are still in range.
//
// For PyPI this is everything under the `.pypi/` prefix — the simple index
// (.pypi/simple.html) and the per-package indexes (.pypi/<pkg>/<pkg>.html) —
// which jfrog.(*adapter).GetPackages/GetVersions read to list packages and
// versions.
func IsPackageIndexFile(artifactType types.ArtifactType, uri string) bool {
	normalized := strings.TrimPrefix(uri, "/")
	switch artifactType {
	case types.PYTHON:
		return strings.HasPrefix(normalized, ".pypi/")
	default:
		return false
	}
}

func ValidateDateFilter(df *types.DateFilter) error {

	if df.Match != types.DateFilterMatchAny && df.Match != types.DateFilterMatchAll {
		log.Error().Msgf("dateFilter.match must be 'ANY' or 'ALL', got %q", df.Match)
		return fmt.Errorf("dateFilter.match must be 'ANY' or 'ALL', got %q", df.Match)
	}

	if df.CreatedAfter == nil && df.DownloadedAfter == nil {
		log.Error().Msg("dateFilter is present but neither createdAfter nor downloadedAfter is specified")
		return fmt.Errorf("dateFilter is present but neither createdAfter nor downloadedAfter is specified")
	}

	return nil
}
func AddPackageErrorToStat(stats *types.TransferStats, pkg types.Package, srcRegistry string, err error) {
	stat := types.FileStat{
		Name:     pkg.Name,
		Registry: srcRegistry,
		Uri:      pkg.URL,
		Size:     int64(pkg.Size),
		Status:   types.StatusFail,
		Error:    err.Error(),
	}
	stats.Add(stat)
}
func AddFileErrorToStat(stats *types.TransferStats, file *types.File, srcRegistry string, err error) {
	stat := types.FileStat{
		Name:     file.Name,
		Registry: srcRegistry,
		Uri:      file.Uri,
		Size:     int64(file.Size),
		Status:   types.StatusFail,
		Error:    err.Error(),
	}
	stats.Add(stat)
}
