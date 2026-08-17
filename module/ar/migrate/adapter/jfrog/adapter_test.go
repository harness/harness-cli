package jfrog

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestChartRepoRelPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "relative path remains unchanged",
			in:   "team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
		{
			name: "leading slash is trimmed",
			in:   "/team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
		{
			name: "absolute artifactory URL drops repo prefix",
			in:   "https://jfrog.example/artifactory/helm-http-local/team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
		{
			name: "absolute artifactory path drops repo prefix",
			in:   "/artifactory/helm-http-local/team-a/abc-1.0.1.tgz",
			want: "team-a/abc-1.0.1.tgz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chartRepoRelPath(tt.in)
			if got != tt.want {
				t.Errorf("chartRepoRelPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetNestedName(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		urls        []string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "relative URL keeps flat package name",
			packageName: "nginx",
			urls:        []string{"nginx-1.0.0.tgz"},
			wantName:    "nginx",
		},
		{
			name:        "absolute URL carries nested prefix",
			packageName: "nginx",
			urls:        []string{"https://jfrog.example/artifactory/helm-http-local/team-a/nginx-1.0.0.tgz"},
			wantName:    "team-a/nginx",
		},
		{
			name:        "empty URL list errors",
			packageName: "nginx",
			urls:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNestedName(tt.packageName, tt.urls)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantName {
				t.Errorf("getNestedName(%q, %v) = %q, want %q", tt.packageName, tt.urls, got, tt.wantName)
			}
		})
	}
}

func TestSampleStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		n    int
		want []string
	}{
		{name: "shorter than limit returns all", in: []string{"a", "b"}, n: 20, want: []string{"a", "b"}},
		{name: "longer than limit is truncated", in: []string{"a", "b", "c"}, n: 2, want: []string{"a", "b"}},
		{name: "empty returns empty", in: []string{}, n: 20, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sampleStrings(tt.in, tt.n)
			if len(got) != len(tt.want) {
				t.Fatalf("sampleStrings(%v, %d) len = %d, want %d", tt.in, tt.n, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sampleStrings(%v, %d)[%d] = %q, want %q", tt.in, tt.n, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFileURIs(t *testing.T) {
	files := make([]types.File, 0, logSampleLimit+5)
	for i := 0; i < logSampleLimit+5; i++ {
		files = append(files, types.File{Uri: fmt.Sprintf("dir/file-%02d.bin", i)})
	}

	got := fileURIs(files, logSampleLimit)
	if len(got) != logSampleLimit {
		t.Fatalf("fileURIs len = %d, want %d", len(got), logSampleLimit)
	}
	if got[logSampleLimit-1] != "dir/file-19.bin" {
		t.Errorf("fileURIs last sample = %q, want %q", got[logSampleLimit-1], "dir/file-19.bin")
	}

	if got := fileURIs(files[:3], logSampleLimit); len(got) != 3 {
		t.Errorf("fileURIs with fewer files than limit len = %d, want 3", len(got))
	}
}

func TestSearchedFilePaths(t *testing.T) {
	files := []types.SearchedFile{
		{Path: "com/acme", Name: "a.jar"},
		{Path: ".", Name: "root.bin"},
	}
	got := searchedFilePaths(files, logSampleLimit)
	if len(got) != 2 {
		t.Fatalf("searchedFilePaths len = %d, want 2", len(got))
	}
	if got[0] != "com/acme/a.jar" {
		t.Errorf("searchedFilePaths[0] = %q, want %q", got[0], "com/acme/a.jar")
	}
	if got[1] != "root.bin" {
		t.Errorf("searchedFilePaths[1] = %q, want %q", got[1], "root.bin")
	}
}

type stubFilesClient struct {
	Client
	files []types.File
}

func (s stubFilesClient) GetFiles(registry string) ([]types.File, error) { return s.files, nil }

func makeSizedFiles(n int) []types.File {
	files := make([]types.File, 0, n)
	for i := 0; i < n; i++ {
		files = append(files, types.File{Uri: fmt.Sprintf("dir/file-%02d.bin", i), Size: i})
	}
	return files
}

func captureLog(t *testing.T, level zerolog.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := log.Logger
	log.Logger = zerolog.New(buf).Level(level)
	t.Cleanup(func() { log.Logger = prev })
	return buf
}

func TestGetFilesInfoLogIsBounded(t *testing.T) {
	buf := captureLog(t, zerolog.InfoLevel)
	a := NewAdapterWithClient(types.RegistryConfig{}, stubFilesClient{files: makeSizedFiles(logSampleLimit + 5)})

	if _, err := a.GetFiles("generic-local"); err != nil {
		t.Fatalf("GetFiles: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "25 file(s)") {
		t.Errorf("info log missing total count: %q", out)
	}
	if !strings.Contains(out, "first 20") {
		t.Errorf("info log missing sample bound: %q", out)
	}
	for _, leaked := range []string{"file-20", "file-21", "file-22", "file-23", "file-24"} {
		if strings.Contains(out, leaked) {
			t.Errorf("info log leaked entry past sample bound: %q", leaked)
		}
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 0 {
		t.Errorf("expected a single info log line, got %d lines", lines+1)
	}
}

func TestGetFilesDebugLogEmitsOneRecordPerFile(t *testing.T) {
	buf := captureLog(t, zerolog.DebugLevel)
	const total = logSampleLimit + 5
	a := NewAdapterWithClient(types.RegistryConfig{}, stubFilesClient{files: makeSizedFiles(total)})

	if _, err := a.GetFiles("generic-local"); err != nil {
		t.Fatalf("GetFiles: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != total+1 {
		t.Fatalf("expected 1 bounded info line + %d per-file debug records, got %d lines", total, len(lines))
	}
	for i, line := range lines[1:] {
		wantURI := fmt.Sprintf(`"uri":"dir/file-%02d.bin"`, i)
		if !strings.Contains(line, wantURI) {
			t.Errorf("debug record %d missing %q: %q", i, wantURI, line)
		}
		wantSize := fmt.Sprintf(`"size":%d`, i)
		if !strings.Contains(line, wantSize) {
			t.Errorf("debug record %d missing %q: %q", i, wantSize, line)
		}
	}
}
