//go:build e2e

// Package filters contains end-to-end migration tests for include/exclude
// pattern filtering and time-based (dateFilter) filtering. Each test asserts
// both that the matched artifacts land AND that the excluded ones are absent, so
// a filter that silently matches everything (or nothing) cannot pass.
package filters

import (
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/tests/harness"
)

func mustTime(t *testing.T, s string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad time %q: %v", s, err)
	}
	return &parsed
}

// TestRawFileLevelIncludePatterns keeps only configs/** and asserts the deep
// config file lands while an unmatched asset does not.
func TestRawFileLevelIncludePatterns(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:       "RAW",
		PackageType:        "GENERIC",
		SourceRegistry:     "raw-local",
		DestRegistry:       harness.UniqueRegistry(t, "filtrawinc"),
		IncludePatterns:    []string{"configs/**"},
		ExpectedRawURIs:    []string{"configs/v1/config.yaml"},
		NotExpectedRawURIs: []string{"assets/images/logo.png"},
	})
}

// TestRawFileLevelExcludePatterns drops assets/** and asserts the excluded file
// is absent while the rest lands.
func TestRawFileLevelExcludePatterns(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:       "RAW",
		PackageType:        "GENERIC",
		SourceRegistry:     "raw-local",
		DestRegistry:       harness.UniqueRegistry(t, "filtrawexc"),
		ExcludePatterns:    []string{"assets/**"},
		ExpectedRawURIs:    []string{"configs/v1/config.yaml"},
		NotExpectedRawURIs: []string{"assets/images/logo.png"},
	})
}

// TestGenericFileLevelIncludePatterns keeps only data/** in a GENERIC repo.
// GENERIC uploads land at default/default/<uri>.
func TestGenericFileLevelIncludePatterns(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:       "GENERIC",
		PackageType:        "GENERIC",
		SourceRegistry:     "generic-local",
		DestRegistry:       harness.UniqueRegistry(t, "filtgeninc"),
		IncludePatterns:    []string{"data/**"},
		ExpectedRawURIs:    []string{"default/default/data/config.json"},
		NotExpectedRawURIs: []string{"default/default/bin/run.sh"},
	})
}

// TestSwiftPackageLevelIncludePatterns keeps only the myscope.harness package
// and asserts the swift.trial package is absent (package-level filtering).
func TestSwiftPackageLevelIncludePatterns(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:    "SWIFT",
		PackageType:     "SWIFT",
		SourceRegistry:  "swift-local",
		DestRegistry:    harness.UniqueRegistry(t, "filtswiftpkg"),
		IncludePatterns: []string{"myscope.harness"},
		ExpectedFiles: []harness.ExpectedFile{
			{Pkg: "myscope.harness", Version: "1.0.0"},
		},
		NotExpectedFiles: []harness.ExpectedFile{
			{Pkg: "swift.trial", Version: "1.0.2"},
		},
	})
}

// TestDeepGlobMatch exercises a ** glob that matches a nested path.
func TestDeepGlobMatch(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:       "RAW",
		PackageType:        "GENERIC",
		SourceRegistry:     "raw-local",
		DestRegistry:       harness.UniqueRegistry(t, "filtdeepglob"),
		IncludePatterns:    []string{"assets/**/*.png"},
		ExpectedRawURIs:    []string{"assets/images/logo.png"},
		NotExpectedRawURIs: []string{"configs/v1/config.yaml"},
	})
}

// TestFiltersReduceToZero asserts a filter that matches nothing yields zero
// migrated files and nothing lands (no false-positive success).
func TestFiltersReduceToZero(t *testing.T) {
	creds := harness.RequireEnv(t)
	_ = harness.BuildBinary(t) // ensure fixtures generated for the in-process path

	harness.RunExpectZeroFiles(t, creds, harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC",
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "filtzero"),
		IncludePatterns: []string{"does-not-exist/**"},
		NotExpectedRawURIs: []string{
			"configs/v1/config.yaml",
			"assets/images/logo.png",
		},
	})
}

// TestDateFilterAny (match ANY, createdAfter) keeps files created on/after the
// threshold. config.yaml (2024-02-20) is kept and lands; logo.png (2024-01-10)
// is filtered out and absent.
func TestDateFilterAny(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:   "RAW",
		PackageType:    "GENERIC",
		SourceRegistry: "raw-local",
		DestRegistry:   harness.UniqueRegistry(t, "filtdateany"),
		DateFilter: &types.DateFilter{
			Match:        types.DateFilterMatchAny,
			CreatedAfter: mustTime(t, "2024-02-01T00:00:00Z"),
		},
		ExpectedRawURIs:    []string{"configs/v1/config.yaml"},
		NotExpectedRawURIs: []string{"assets/images/logo.png"},
	})
}

// TestDateFilterAll (match ALL, createdAfter + downloadedAfter) keeps files that
// satisfy BOTH. Only config.yaml (created 2024-02-20, downloaded 2026-06-01)
// qualifies; app-1.0 (downloaded 2020) and logo.png (created 2024-01-10) drop.
func TestDateFilterAll(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:   "RAW",
		PackageType:    "GENERIC",
		SourceRegistry: "raw-local",
		DestRegistry:   harness.UniqueRegistry(t, "filtdateall"),
		DateFilter: &types.DateFilter{
			Match:           types.DateFilterMatchAll,
			CreatedAfter:    mustTime(t, "2024-02-01T00:00:00Z"),
			DownloadedAfter: mustTime(t, "2026-01-01T00:00:00Z"),
		},
		ExpectedRawURIs:    []string{"configs/v1/config.yaml"},
		NotExpectedRawURIs: []string{"assets/images/logo.png"},
	})
}

// TestDateFilterPlusInclude combines a date filter with an include pattern and
// asserts the intersection: config.yaml satisfies both; logo.png fails both.
func TestDateFilterPlusInclude(t *testing.T) {
	creds := harness.RequireEnv(t)
	bin := harness.BuildBinary(t)

	harness.RunExpectAbsent(t, bin, creds, harness.Spec{
		ArtifactType:    "RAW",
		PackageType:     "GENERIC",
		SourceRegistry:  "raw-local",
		DestRegistry:    harness.UniqueRegistry(t, "filtdateinc"),
		IncludePatterns: []string{"configs/**"},
		DateFilter: &types.DateFilter{
			Match:        types.DateFilterMatchAny,
			CreatedAfter: mustTime(t, "2024-02-01T00:00:00Z"),
		},
		ExpectedRawURIs:    []string{"configs/v1/config.yaml"},
		NotExpectedRawURIs: []string{"assets/images/logo.png"},
	})
}
