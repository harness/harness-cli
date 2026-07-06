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

func FilterFilesByTime(searchedFiles []types.SearchedFile, mapping *types.RegistryMapping) map[string]struct{} {
	result := map[string]struct{}{}

	if mapping.IncludeCreatedAfter != nil {
		log.Info().Msgf("Filtering files by IncludeCreatedAfter: %v", mapping.IncludeCreatedAfter)
		skipped := 0
		for _, f := range searchedFiles {
			created, err := parseDate(f.Created)
			if err != nil {
				skipped++
				log.Warn().Msgf("Skipping file %s: failed to parse created date %q: %v", f.Name, f.Created, err)
				continue
			}
			if onOrAfter(created, *mapping.IncludeCreatedAfter) {
				uri := buildURI(f.Path, f.Name)
				result[uri] = struct{}{}
			}
		}
		if skipped > 0 {
			log.Warn().Msgf("%d file(s) skipped due to invalid or missing created date", skipped)
		}
		return result
	}

	if mapping.IncludeAccessedAfter != nil {
		log.Info().Msgf("Filtering files by IncludeAccessedAfter: %v", mapping.IncludeAccessedAfter)
		skipped := 0
		for _, f := range searchedFiles {
			for _, stat := range f.Stats {
				downloaded, err := parseDate(stat.Downloaded)
				if err != nil {
					skipped++
					log.Warn().Msgf("Skipping file %s: failed to parse downloaded date %q: %v", f.Name, stat.Downloaded, err)
					continue
				}
				if onOrAfter(downloaded, *mapping.IncludeAccessedAfter) {
					uri := buildURI(f.Path, f.Name)
					result[uri] = struct{}{}
					break
				}
			}
		}
		if skipped > 0 {
			log.Warn().Msgf("%d file(s) skipped due to invalid or missing downloaded date ,Please use IncludeCreatedAfter to migrate them", skipped)
		}
		return result
	}

	return result
}
