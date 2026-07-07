package util

import (
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/stretchr/testify/assert"
)

// ── parseDate ────────────────────────────────────────────────────────────────

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantUTC string
	}{
		{
			name:    "RFC3339Nano with Z",
			input:   "2026-06-22T18:00:12.881Z",
			wantUTC: "2026-06-22T18:00:12.881Z",
		},
		{
			name:    "RFC3339 with Z",
			input:   "2026-06-22T18:00:12Z",
			wantUTC: "2026-06-22T18:00:12Z",
		},
		{
			name:    "milliseconds with Z offset layout",
			input:   "2023-01-01T00:00:00.000Z",
			wantUTC: "2023-01-01T00:00:00Z",
		},
		{
			name:    "RFC3339 with positive offset",
			input:   "2023-03-01T10:00:00+05:30",
			wantUTC: "2023-03-01T04:30:00Z",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid date",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "date only (no time)",
			input:   "2026-06-22",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDate(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantUTC, got.UTC().Format(time.RFC3339Nano))
		})
	}
}

// ── buildURI ─────────────────────────────────────────────────────────────────

func TestBuildURI(t *testing.T) {
	tests := []struct {
		name string
		path string
		file string
		want string
	}{
		{
			name: "normal path and name",
			path: "foo/company.grpc.pkg/1.0.0",
			file: "company.grpc.pkg.1.0.0.nupkg",
			want: "/foo/company.grpc.pkg/1.0.0/company.grpc.pkg.1.0.0.nupkg",
		},
		{
			name: "path with leading slash",
			path: "/foo/bar/1.0.0",
			file: "file.txt",
			want: "/foo/bar/1.0.0/file.txt",
		},
		{
			name: "path with trailing slash",
			path: "foo/bar/",
			file: "file.txt",
			want: "/foo/bar/file.txt",
		},
		{
			name: "empty path",
			path: "",
			file: "file.txt",
			want: "/file.txt",
		},
		{
			name: "dot path",
			path: ".",
			file: "file.txt",
			want: "/file.txt",
		},
		{
			name: "path with leading and trailing slashes",
			path: "/foo/bar/",
			file: "archive.zip",
			want: "/foo/bar/archive.zip",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildURI(tc.path, tc.file)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ── onOrAfter ────────────────────────────────────────────────────────────────

func TestOnOrAfter(t *testing.T) {
	threshold := time.Date(2026, 6, 1, 18, 0, 11, 0, time.UTC)

	tests := []struct {
		name      string
		t         time.Time
		threshold time.Time
		want      bool
	}{
		{
			name:      "strictly after threshold",
			t:         time.Date(2026, 6, 22, 18, 0, 12, 0, time.UTC),
			threshold: threshold,
			want:      true,
		},
		{
			name:      "exactly equal to threshold",
			t:         threshold,
			threshold: threshold,
			want:      true,
		},
		{
			name:      "strictly before threshold",
			t:         time.Date(2026, 5, 1, 18, 0, 11, 0, time.UTC),
			threshold: threshold,
			want:      false,
		},
		{
			name:      "one nanosecond before threshold",
			t:         threshold.Add(-1),
			threshold: threshold,
			want:      false,
		},
		{
			name:      "one nanosecond after threshold",
			t:         threshold.Add(1),
			threshold: threshold,
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, onOrAfter(tc.t, tc.threshold))
		})
	}
}

// ── FilterFilesByDate ─────────────────────────────────────────────────────────

func TestFilterFilesByDate(t *testing.T) {
	allFiles := []types.File{
		{Name: "file-a.nupkg", Uri: "/foo/1.0.0/file-a.nupkg"},
		{Name: "file-b.nupkg", Uri: "/foo/2.0.0/file-b.nupkg"},
		{Name: "file-c.nupkg", Uri: "/foo/3.0.0/file-c.nupkg"},
	}

	tests := []struct {
		name         string
		filteredURIs map[string]struct{}
		wantNames    []string
	}{
		{
			name: "keep two of three",
			filteredURIs: map[string]struct{}{
				"/foo/1.0.0/file-a.nupkg": {},
				"/foo/3.0.0/file-c.nupkg": {},
			},
			wantNames: []string{"file-a.nupkg", "file-c.nupkg"},
		},
		{
			name:         "empty filtered set returns nothing",
			filteredURIs: map[string]struct{}{},
			wantNames:    nil,
		},
		{
			name: "URI not in files list is ignored",
			filteredURIs: map[string]struct{}{
				"/foo/99.0.0/nonexistent.nupkg": {},
			},
			wantNames: nil,
		},
		{
			name: "all files kept",
			filteredURIs: map[string]struct{}{
				"/foo/1.0.0/file-a.nupkg": {},
				"/foo/2.0.0/file-b.nupkg": {},
				"/foo/3.0.0/file-c.nupkg": {},
			},
			wantNames: []string{"file-a.nupkg", "file-b.nupkg", "file-c.nupkg"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterFilesByDate(allFiles, tc.filteredURIs)
			var names []string
			for _, f := range got {
				names = append(names, f.Name)
			}
			assert.Equal(t, tc.wantNames, names)
		})
	}
}

// ── FilterFilesByTime ─────────────────────────────────────────────────────────

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptrTime(t time.Time) *time.Time { return &t }

var testSearchedFiles = []types.SearchedFile{
	{
		Path:    "foo/pkg/1.0.0",
		Name:    "pkg.1.0.0.nupkg",
		Created: "2020-01-01T00:00:00Z",
		Stats:   []types.DownloadStat{{Downloaded: "2026-04-01T17:58:40Z"}},
	},
	{
		Path:    "foo/pkg/1.0.0",
		Name:    "pkg.1.0.0.nuspec",
		Created: "2023-01-01T00:00:00Z",
		Stats:   []types.DownloadStat{{Downloaded: "2022-06-22T17:58:42Z"}},
	},
	{
		Path:    "foo/pkg/2.0.0",
		Name:    "pkg.2.0.0.nupkg",
		Created: "2020-02-01T00:00:00Z",
		Stats:   []types.DownloadStat{{Downloaded: "2026-03-01T18:00:10Z"}},
	},
	{
		Path:    "foo/pkg/2.0.0",
		Name:    "pkg.2.0.0.nupkg.sha512",
		Created: "2023-03-01T00:00:00Z",
		Stats:   []types.DownloadStat{{Downloaded: "2026-06-22T18:00:12Z"}},
	},
	{
		Path:    "foo/pkg/2.0.0",
		Name:    "pkg.2.0.0.nuspec",
		Created: "2023-03-01T00:00:00Z",
		Stats:   []types.DownloadStat{{Downloaded: "2026-06-22T18:00:13Z"}},
	},
}

func TestFilterFilesByTime_NoFilter(t *testing.T) {
	mapping := &types.RegistryMapping{}
	got := CreateMapOfFilteredFile(testSearchedFiles, mapping)
	assert.Empty(t, got, "no filter should return empty map")
}

func TestFilterFilesByTime_IncludeCreatedAfter(t *testing.T) {
	tests := []struct {
		name      string
		threshold time.Time
		wantURIs  []string
	}{
		{
			name:      "threshold before all — all 5 match",
			threshold: mustTime("2019-01-01T00:00:00Z"),
			wantURIs: []string{
				"/foo/pkg/1.0.0/pkg.1.0.0.nupkg",
				"/foo/pkg/1.0.0/pkg.1.0.0.nuspec",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
				"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
			},
		},
		{
			name:      "threshold at exact created time — exact match included",
			threshold: mustTime("2020-01-01T00:00:00Z"),
			wantURIs: []string{
				"/foo/pkg/1.0.0/pkg.1.0.0.nupkg",
				"/foo/pkg/1.0.0/pkg.1.0.0.nuspec",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
				"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
			},
		},
		{
			name:      "threshold after two early files — 3 match",
			threshold: mustTime("2022-01-01T00:00:00Z"),
			wantURIs: []string{
				"/foo/pkg/1.0.0/pkg.1.0.0.nuspec",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
				"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
			},
		},
		{
			name:      "threshold after all files — none match",
			threshold: mustTime("2025-01-01T00:00:00Z"),
			wantURIs:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mapping := &types.RegistryMapping{DateFilter: &types.DateFilter{Match: types.DateFilterMatchAny, CreatedAfter: ptrTime(tc.threshold)}}
			got := CreateMapOfFilteredFile(testSearchedFiles, mapping)
			var uris []string
			for u := range got {
				uris = append(uris, u)
			}
			assert.ElementsMatch(t, tc.wantURIs, uris)
		})
	}
}

func TestFilterFilesByTime_IncludeAccessedAfter(t *testing.T) {
	tests := []struct {
		name      string
		threshold time.Time
		wantURIs  []string
	}{
		{
			name:      "threshold before all downloads — all 5 match",
			threshold: mustTime("2020-01-01T00:00:00Z"),
			wantURIs: []string{
				"/foo/pkg/1.0.0/pkg.1.0.0.nupkg",
				"/foo/pkg/1.0.0/pkg.1.0.0.nuspec",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg",
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
				"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
			},
		},
		{
			name:      "threshold June 2026 — 2 match",
			threshold: mustTime("2026-06-01T18:00:11Z"),
			wantURIs: []string{
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
				"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
			},
		},
		{
			name:      "threshold exactly equals a download — that file included",
			threshold: mustTime("2026-06-22T18:00:12Z"),
			wantURIs: []string{
				"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
				"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
			},
		},
		{
			name:      "threshold after all downloads — none match",
			threshold: mustTime("2027-01-01T00:00:00Z"),
			wantURIs:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mapping := &types.RegistryMapping{DateFilter: &types.DateFilter{Match: types.DateFilterMatchAny, DownloadedAfter: ptrTime(tc.threshold)}}
			got := CreateMapOfFilteredFile(testSearchedFiles, mapping)
			var uris []string
			for u := range got {
				uris = append(uris, u)
			}
			assert.ElementsMatch(t, tc.wantURIs, uris)
		})
	}
}

func TestFilterFilesByTime_InvalidDateSkipped(t *testing.T) {
	files := []types.SearchedFile{
		{Path: "a/1.0.0", Name: "a.nupkg", Created: "not-a-date", Stats: []types.DownloadStat{{Downloaded: "also-bad"}}},
		{Path: "b/1.0.0", Name: "b.nupkg", Created: "2021-01-01T00:00:00Z", Stats: []types.DownloadStat{{Downloaded: "bad-date"}}},
		{Path: "c/1.0.0", Name: "c.nupkg", Created: "2021-06-01T00:00:00Z", Stats: []types.DownloadStat{{Downloaded: "2026-07-01T00:00:00Z"}}},
	}
	threshold := mustTime("2020-01-01T00:00:00Z")

	t.Run("createdAfter skips invalid created date", func(t *testing.T) {
		mapping := &types.RegistryMapping{DateFilter: &types.DateFilter{Match: types.DateFilterMatchAny, CreatedAfter: ptrTime(threshold)}}
		got := CreateMapOfFilteredFile(files, mapping)
		// "a.nupkg" has bad created → skipped; "b" and "c" have valid created → matched
		assert.Contains(t, got, "/b/1.0.0/b.nupkg")
		assert.Contains(t, got, "/c/1.0.0/c.nupkg")
		assert.NotContains(t, got, "/a/1.0.0/a.nupkg")
	})

	t.Run("downloadedAfter skips invalid downloaded date", func(t *testing.T) {
		mapping := &types.RegistryMapping{DateFilter: &types.DateFilter{Match: types.DateFilterMatchAny, DownloadedAfter: ptrTime(threshold)}}
		got := CreateMapOfFilteredFile(files, mapping)
		// "a.nupkg" bad downloaded → skipped; "b.nupkg" bad downloaded → skipped; "c.nupkg" valid → matched
		assert.Contains(t, got, "/c/1.0.0/c.nupkg")
		assert.NotContains(t, got, "/a/1.0.0/a.nupkg")
		assert.NotContains(t, got, "/b/1.0.0/b.nupkg")
	})
}

func TestFilterFilesByTime_AnyMatch_BothFilters(t *testing.T) {
	// createdAfter=2023-01-01: nuspec(1.0.0), nupkg.sha512(2.0.0), nuspec(2.0.0) — 3 files
	// downloadedAfter=2026-06-01: nupkg.sha512(2.0.0), nuspec(2.0.0) — 2 files
	// ANY union: 3 unique files
	mapping := &types.RegistryMapping{DateFilter: &types.DateFilter{
		Match:           types.DateFilterMatchAny,
		CreatedAfter:    ptrTime(mustTime("2023-01-01T00:00:00Z")),
		DownloadedAfter: ptrTime(mustTime("2026-06-01T00:00:00Z")),
	}}
	got := CreateMapOfFilteredFile(testSearchedFiles, mapping)
	var uris []string
	for u := range got {
		uris = append(uris, u)
	}
	assert.ElementsMatch(t, []string{
		"/foo/pkg/1.0.0/pkg.1.0.0.nuspec",
		"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
		"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
	}, uris)
}

func TestFilterFilesByTime_AllMatch_BothFilters(t *testing.T) {
	// createdAfter=2023-01-01: nuspec(1.0.0), nupkg.sha512(2.0.0), nuspec(2.0.0)
	// downloadedAfter=2026-06-01: nupkg.sha512(2.0.0), nuspec(2.0.0)
	// ALL intersection: nupkg.sha512(2.0.0), nuspec(2.0.0) — 2 files
	mapping := &types.RegistryMapping{DateFilter: &types.DateFilter{
		Match:           types.DateFilterMatchAll,
		CreatedAfter:    ptrTime(mustTime("2023-01-01T00:00:00Z")),
		DownloadedAfter: ptrTime(mustTime("2026-06-01T00:00:00Z")),
	}}
	got := CreateMapOfFilteredFile(testSearchedFiles, mapping)
	var uris []string
	for u := range got {
		uris = append(uris, u)
	}
	assert.ElementsMatch(t, []string{
		"/foo/pkg/2.0.0/pkg.2.0.0.nupkg.sha512",
		"/foo/pkg/2.0.0/pkg.2.0.0.nuspec",
	}, uris)
}

// ── ValidateDateFilter ────────────────────────────────────────────────────────

func TestValidateDateFilter(t *testing.T) {
	createdAfter := mustTime("2024-01-01T00:00:00Z")
	downloadedAfter := mustTime("2024-06-01T00:00:00Z")

	tests := []struct {
		name    string
		df      *types.DateFilter
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid ANY with createdAfter only",
			df:      &types.DateFilter{Match: types.DateFilterMatchAny, CreatedAfter: &createdAfter},
			wantErr: false,
		},
		{
			name:    "valid ANY with downloadedAfter only",
			df:      &types.DateFilter{Match: types.DateFilterMatchAny, DownloadedAfter: &downloadedAfter},
			wantErr: false,
		},
		{
			name:    "valid ANY with both dates",
			df:      &types.DateFilter{Match: types.DateFilterMatchAny, CreatedAfter: &createdAfter, DownloadedAfter: &downloadedAfter},
			wantErr: false,
		},
		{
			name:    "valid ALL with createdAfter only",
			df:      &types.DateFilter{Match: types.DateFilterMatchAll, CreatedAfter: &createdAfter},
			wantErr: false,
		},
		{
			name:    "valid ALL with both dates",
			df:      &types.DateFilter{Match: types.DateFilterMatchAll, CreatedAfter: &createdAfter, DownloadedAfter: &downloadedAfter},
			wantErr: false,
		},
		{
			name:    "invalid match value",
			df:      &types.DateFilter{Match: "INVALID", CreatedAfter: &createdAfter},
			wantErr: true,
			errMsg:  "dateFilter.match must be 'ANY' or 'ALL'",
		},
		{
			name:    "empty match value",
			df:      &types.DateFilter{Match: "", CreatedAfter: &createdAfter},
			wantErr: true,
			errMsg:  "dateFilter.match must be 'ANY' or 'ALL'",
		},
		{
			name:    "no dates provided",
			df:      &types.DateFilter{Match: types.DateFilterMatchAny},
			wantErr: true,
			errMsg:  "neither createdAfter nor downloadedAfter is specified",
		},
		{
			name:    "invalid match and no dates",
			df:      &types.DateFilter{Match: "NONE"},
			wantErr: true,
			errMsg:  "dateFilter.match must be 'ANY' or 'ALL'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDateFilter(tt.df)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
