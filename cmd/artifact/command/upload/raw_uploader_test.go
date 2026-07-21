package upload

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	commonupload "github.com/harness/harness-cli/util/common/upload"
)

// rawDestPaths extracts the DestPath from each RawUploadJob and returns them sorted.
func rawDestPaths(t *testing.T, jobs []commonupload.FileUploadJob) []string {
	t.Helper()
	out := make([]string, 0, len(jobs))
	for _, j := range jobs {
		rj, ok := j.(*commonupload.RawUploadJob)
		if !ok {
			t.Fatalf("expected *RawUploadJob, got %T", j)
		}
		out = append(out, rj.DestPath)
	}
	sort.Strings(out)
	return out
}

// ── resolveRawDestPath ────────────────────────────────────────────────────────

func TestResolveRawDestPath(t *testing.T) {
	tests := []struct {
		name     string
		template string
		captures []string
		relPath  string
		want     string
	}{
		{
			name:     "no captures, flat file",
			template: "uploads",
			captures: nil,
			relPath:  "file.txt",
			want:     "uploads/file.txt",
		},
		{
			name:     "no captures, trailing slash in template",
			template: "uploads/",
			captures: nil,
			relPath:  "file.txt",
			want:     "uploads/file.txt",
		},
		{
			name:     "no captures, preserves subdirectory structure",
			template: "data",
			captures: nil,
			relPath:  "subdir/nested/file.bin",
			want:     "data/subdir/nested/file.bin",
		},
		{
			name:     "single capture, only basename appended",
			template: "builds/{1}",
			captures: []string{"linux"},
			relPath:  "linux/app.zip",
			want:     "builds/linux/app.zip",
		},
		{
			name:     "two captures, only basename appended",
			template: "releases/{1}/{2}",
			captures: []string{"linux", "amd64"},
			relPath:  "linux/amd64/binary",
			want:     "releases/linux/amd64/binary",
		},
		{
			name:     "empty captures slice treated as no captures",
			template: "files",
			captures: []string{},
			relPath:  "sub/file.tar.gz",
			want:     "files/sub/file.tar.gz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveRawDestPath(tc.template, tc.captures, tc.relPath)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── GetRegistryAndPath ────────────────────────────────────────────────────────

func TestRawGetRegistryAndPath_Valid(t *testing.T) {
	u := &RawUploader{}
	reg, err := u.GetRegistryAndPath("my-raw-registry/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg != "my-raw-registry" {
		t.Errorf("registryName: got %q, want my-raw-registry", reg)
	}
	if u.RegistryName != "my-raw-registry" {
		t.Errorf("u.RegistryName: got %q, want my-raw-registry", u.RegistryName)
	}
	if u.DestTemplate != "some/path" {
		t.Errorf("u.DestTemplate: got %q, want some/path", u.DestTemplate)
	}
}

func TestRawGetRegistryAndPath_NoSlash(t *testing.T) {
	u := &RawUploader{}
	_, err := u.GetRegistryAndPath("no-slash-here")
	if err == nil {
		t.Fatal("expected error for target without '/'")
	}
	if !strings.Contains(err.Error(), "<registry>/<path>") {
		t.Errorf("error should describe expected format, got: %v", err)
	}
}

func TestRawGetRegistryAndPath_SetsStateForGetFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.bin"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	u := &RawUploader{
		SrcPattern: filepath.Join(dir, "*.bin"),
	}

	reg, err := u.GetRegistryAndPath("raw-reg/uploads")
	if err != nil {
		t.Fatalf("GetRegistryAndPath: %v", err)
	}
	if reg != "raw-reg" {
		t.Errorf("registry: got %q, want raw-reg", reg)
	}

	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	rj := jobs[0].(*commonupload.RawUploadJob)
	if rj.RegistryName != "raw-reg" {
		t.Errorf("job.RegistryName: got %q, want raw-reg", rj.RegistryName)
	}
}

// ── GetFiles – literal path ───────────────────────────────────────────────────

func TestRawGetFiles_LiteralFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	u := &RawUploader{
		SrcPattern:   path,
		DestTemplate: "documents",
		RegistryName: "raw-reg",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 1 {
		t.Errorf("FileCount: got %d, want 1", stats.FileCount)
	}

	got := rawDestPaths(t, jobs)
	want := []string{"documents/report.pdf"}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestRawGetFiles_LiteralFile_NotRegular(t *testing.T) {
	dir := t.TempDir()
	u := &RawUploader{
		SrcPattern:   dir,
		DestTemplate: "repo",
		RegistryName: "raw-reg",
	}
	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for directory as literal path")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error should say 'not a regular file', got: %v", err)
	}
}

func TestRawGetFiles_LiteralFile_NonExistent(t *testing.T) {
	u := &RawUploader{
		SrcPattern:   "/no/such/file.bin",
		DestTemplate: "repo",
		RegistryName: "raw-reg",
	}
	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// ── GetFiles – wildcard patterns ─────────────────────────────────────────────

func TestRawGetFiles_SingleStarPattern_FlatDestination(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.bin":     "data",
		"b.bin":     "data",
		"c.txt":     "text",
		"sub/d.bin": "data",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*.bin"),
		DestTemplate: "files",
		RegistryName: "raw-reg",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2 (single star should not cross dir boundary)", stats.FileCount)
	}

	got := rawDestPaths(t, jobs)
	want := []string{"files/a.bin", "files/b.bin"}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestRawGetFiles_DoubleStarPattern_PreservesStructure(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.bin":       "data",
		"sub/b.bin":   "data",
		"sub/c/d.bin": "data",
		"sub/c/e.txt": "text",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "**/*.bin"),
		DestTemplate: "assets",
		RegistryName: "raw-reg",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 3 {
		t.Errorf("FileCount: got %d, want 3", stats.FileCount)
	}

	got := rawDestPaths(t, jobs)
	want := []string{
		"assets/a.bin",
		"assets/sub/b.bin",
		"assets/sub/c/d.bin",
	}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestRawGetFiles_CaptureGroups_UsesBasenameOnly(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"linux/app.bin":   "data",
		"darwin/app.bin":  "data",
		"windows/app.txt": "data",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "(*)/app.bin"),
		DestTemplate: "builds/{1}",
		RegistryName: "raw-reg",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2", stats.FileCount)
	}

	got := rawDestPaths(t, jobs)
	want := []string{
		"builds/darwin/app.bin",
		"builds/linux/app.bin",
	}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestRawGetFiles_NoMatch(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"readme.txt": "ok",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*.bin"),
		DestTemplate: "assets",
		RegistryName: "raw-reg",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
	if stats.FileCount != 0 {
		t.Errorf("expected FileCount=0, got %d", stats.FileCount)
	}
}

func TestRawGetFiles_Stats_TotalBytes(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.bin": "12345",   // 5 bytes
		"b.bin": "1234567", // 7 bytes
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*.bin"),
		DestTemplate: "files",
		RegistryName: "raw-reg",
	}
	_, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2", stats.FileCount)
	}
	if stats.TotalBytes != 12 {
		t.Errorf("TotalBytes: got %d, want 12", stats.TotalBytes)
	}
}

func TestRawGetFiles_NoVersionInDestPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	u := &RawUploader{
		SrcPattern:   path,
		DestTemplate: "uploads",
		RegistryName: "raw-reg",
	}
	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	rj := jobs[0].(*commonupload.RawUploadJob)
	// Raw dest path must NOT contain any version-like segment between template and filename.
	// Exact expected: "uploads/artifact.bin"
	if rj.DestPath != "uploads/artifact.bin" {
		t.Errorf("expected uploads/artifact.bin, got %q", rj.DestPath)
	}
}
