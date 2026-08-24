package upload

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	commonupload "github.com/harness/harness-cli/util/common/upload"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// slicesEqual reports whether two sorted string slices are equal.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// makeUploadTree creates a temporary directory tree from a map of
// relative-path → file-content entries and returns the root directory.
func makeUploadTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}
	return root
}

// stubJob is a minimal FileUploadJob used for filter tests.
type stubJob struct{ id string }

func (s *stubJob) GetID() string                  { return s.id }
func (s *stubJob) GetFilePath() string            { return s.id }
func (s *stubJob) GetFileSize() int64             { return 0 }
func (s *stubJob) Upload(_ context.Context) error { return nil }

// jobIDs extracts and sorts job IDs from a slice of FileUploadJob.
func jobIDs(jobs []commonupload.FileUploadJob) []string {
	out := make([]string, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.GetID())
	}
	sort.Strings(out)
	return out
}

// makeRawJobs builds a minimal slice of FileUploadJob from a list of IDs,
// enough to exercise include/exclude filtering.
func makeRawJobs(ids ...string) []commonupload.FileUploadJob {
	jobs := make([]commonupload.FileUploadJob, 0, len(ids))
	for _, id := range ids {
		jobs = append(jobs, &stubJob{id: id})
	}
	return jobs
}

// ── matchesIncludeExcludePattern ─────────────────────────────────────────────

func TestMatchesPattern_NoSlashInPattern_MatchesBasename(t *testing.T) {
	if !matchesIncludeExcludePattern("*uploader*", "artifact/command/upload/raw_uploader.go") {
		t.Error("*uploader* should match a file whose basename contains 'uploader'")
	}
}

func TestMatchesPattern_NoSlashInPattern_NoMatch(t *testing.T) {
	if matchesIncludeExcludePattern("*uploader*", "artifact/command/upload/upload.go") {
		t.Error("*uploader* should NOT match a file whose basename does not contain 'uploader'")
	}
}

func TestMatchesPattern_WithSlash_MatchesFullPath(t *testing.T) {
	if !matchesIncludeExcludePattern("artifact/command/upload/*.go", "artifact/command/upload/raw_uploader.go") {
		t.Error("path pattern should match against the full relative path")
	}
}

func TestMatchesPattern_WithSlash_NoMatch(t *testing.T) {
	if matchesIncludeExcludePattern("other/command/upload/*.go", "artifact/command/upload/raw_uploader.go") {
		t.Error("path pattern should NOT match a different directory prefix")
	}
}

func TestMatchesPattern_ExactBasename(t *testing.T) {
	if !matchesIncludeExcludePattern("raw_uploader.go", "some/deep/path/raw_uploader.go") {
		t.Error("exact basename match should succeed")
	}
}

func TestMatchesPattern_StarStar_Basename(t *testing.T) {
	if !matchesIncludeExcludePattern("**/*.go", "a/b/c/file.go") {
		t.Error("**/*.go should match files deep in a tree")
	}
}

// ── applyIncludeExcludeFilter ─────────────────────────────────────────────────

func TestApplyIncludeFilter_BasenameGlob(t *testing.T) {
	jobs := makeRawJobs(
		"cmd/artifact/command/upload/raw_uploader.go",
		"cmd/artifact/command/upload/upload.go",
		"cmd/artifact/command/upload/pusher.go",
	)

	got := applyIncludeExcludeFilter(jobs, []string{"*uploader*"}, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 job after include filter, got %d", len(got))
	}
	if got[0].GetID() != "cmd/artifact/command/upload/raw_uploader.go" {
		t.Errorf("unexpected job ID: %s", got[0].GetID())
	}
}

func TestApplyIncludeFilter_NoMatch(t *testing.T) {
	jobs := makeRawJobs("a/b/c.go", "d/e/f.go")
	got := applyIncludeExcludeFilter(jobs, []string{"*.jar"}, nil)
	if len(got) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(got))
	}
}

func TestApplyExcludeFilter_BasenameGlob(t *testing.T) {
	jobs := makeRawJobs(
		"a/raw_uploader.go",
		"b/upload.go",
		"c/pusher.go",
	)

	got := applyIncludeExcludeFilter(jobs, nil, []string{"*uploader*"})
	ids := jobIDs(got)
	expected := []string{"b/upload.go", "c/pusher.go"}
	if !slicesEqual(ids, expected) {
		t.Errorf("got %v, want %v", ids, expected)
	}
}

func TestApplyIncludeExclude_Combined(t *testing.T) {
	jobs := makeRawJobs(
		"src/raw_uploader.go",
		"src/raw_downloader.go",
		"src/upload.go",
	)

	// include all *raw* files, then exclude *downloader*
	got := applyIncludeExcludeFilter(jobs, []string{"*raw*"}, []string{"*downloader*"})
	if len(got) != 1 {
		t.Fatalf("expected 1 job, got %d", len(got))
	}
	if got[0].GetID() != "src/raw_uploader.go" {
		t.Errorf("unexpected job: %s", got[0].GetID())
	}
}

func TestApplyFilter_NoFilters_ReturnsAll(t *testing.T) {
	jobs := makeRawJobs("a.go", "b.go", "c.go")
	got := applyIncludeExcludeFilter(jobs, nil, nil)
	if len(got) != 3 {
		t.Errorf("expected 3 jobs with no filters, got %d", len(got))
	}
}

// ── splitPatternRoot ──────────────────────────────────────────────────────────

func TestSplitPatternRoot(t *testing.T) {
	tests := []struct {
		pattern        string
		wantRoot       string
		wantRelPattern string
	}{
		{"cmd/*/command/upload/*", "cmd", "*/command/upload/*"},
		{"*.go", ".", "*.go"},
		{"**/*.jar", ".", "**/*.jar"},
		{"src/lib/file.txt", "src/lib/file.txt", ""},
		{"build/**/output/*.zip", "build", "**/output/*.zip"},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			root, rel := splitPatternRoot(tc.pattern)
			if root != tc.wantRoot {
				t.Errorf("root: got %q, want %q", root, tc.wantRoot)
			}
			if rel != tc.wantRelPattern {
				t.Errorf("relPattern: got %q, want %q", rel, tc.wantRelPattern)
			}
		})
	}
}
