package upload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── resolveRawDestPath ────────────────────────────────────────────────────────

func TestResolveRawDestPath_EmptyTemplate(t *testing.T) {
	got := resolveRawDestPath("", "subdir/file.txt")
	if got != "subdir/file.txt" {
		t.Errorf("got %q, want %q", got, "subdir/file.txt")
	}
}

func TestResolveRawDestPath_TemplateWithTrailingSlash(t *testing.T) {
	got := resolveRawDestPath("prefix/", "file.bin")
	if got != "prefix/file.bin" {
		t.Errorf("got %q, want %q", got, "prefix/file.bin")
	}
}

func TestResolveRawDestPath_TemplateWithoutTrailingSlash(t *testing.T) {
	got := resolveRawDestPath("prefix", "file.bin")
	if got != "prefix/file.bin" {
		t.Errorf("got %q, want %q", got, "prefix/file.bin")
	}
}

func TestResolveRawDestPath_NestedRelPath(t *testing.T) {
	got := resolveRawDestPath("releases/v1", "linux/amd64/binary")
	if got != "releases/v1/linux/amd64/binary" {
		t.Errorf("got %q, want %q", got, "releases/v1/linux/amd64/binary")
	}
}

func TestResolveRawDestPath_OnlyRelPath_EmptyTemplate(t *testing.T) {
	got := resolveRawDestPath("", "a/b/c.zip")
	if got != "a/b/c.zip" {
		t.Errorf("got %q, want %q", got, "a/b/c.zip")
	}
}

// ── RawUploader.GetRegistryAndPath ────────────────────────────────────────────

func TestGetRegistryAndPath_WithSlash_SplitsCorrectly(t *testing.T) {
	u := &RawUploader{}
	reg, err := u.GetRegistryAndPath("my-registry/path/to/dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg != "my-registry" {
		t.Errorf("registry: got %q, want %q", reg, "my-registry")
	}
	if u.RegistryName != "my-registry" {
		t.Errorf("u.RegistryName: got %q, want %q", u.RegistryName, "my-registry")
	}
	if u.DestTemplate != "path/to/dest" {
		t.Errorf("u.DestTemplate: got %q, want %q", u.DestTemplate, "path/to/dest")
	}
}

func TestGetRegistryAndPath_WithoutSlash_FullNameIsRegistry(t *testing.T) {
	u := &RawUploader{}
	reg, err := u.GetRegistryAndPath("my-registry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg != "my-registry" {
		t.Errorf("registry: got %q, want %q", reg, "my-registry")
	}
	if u.RegistryName != "my-registry" {
		t.Errorf("u.RegistryName: got %q, want %q", u.RegistryName, "my-registry")
	}
	if u.DestTemplate != "" {
		t.Errorf("u.DestTemplate: got %q, want empty", u.DestTemplate)
	}
}

func TestGetRegistryAndPath_SingleSlash_EmptyDest(t *testing.T) {
	u := &RawUploader{}
	reg, err := u.GetRegistryAndPath("my-registry/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg != "my-registry" {
		t.Errorf("registry: got %q, want %q", reg, "my-registry")
	}
	if u.DestTemplate != "" {
		t.Errorf("u.DestTemplate: got %q, want empty", u.DestTemplate)
	}
}

func TestGetRegistryAndPath_SetsRegistryNameField(t *testing.T) {
	u := &RawUploader{}
	_, _ = u.GetRegistryAndPath("reg/dest")
	if u.RegistryName != "reg" {
		t.Errorf("u.RegistryName: got %q, want %q", u.RegistryName, "reg")
	}
}

// ── RawUploader.GetFiles – literal path ───────────────────────────────────────

func TestGetFiles_LiteralPath_SingleFile(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"file.bin": "hello",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "file.bin"),
		RegistryName: "reg",
		DestTemplate: "dest",
	}

	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if stats.FileCount != 1 {
		t.Errorf("FileCount: got %d, want 1", stats.FileCount)
	}
	if stats.TotalBytes != int64(len("hello")) {
		t.Errorf("TotalBytes: got %d, want %d", stats.TotalBytes, len("hello"))
	}
}

func TestGetFiles_LiteralPath_DestPathIncludesTemplate(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"file.bin": "data",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "file.bin"),
		RegistryName: "reg",
		DestTemplate: "releases/v1",
	}

	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	id := jobs[0].GetID()
	if id != "file.bin" {
		t.Errorf("job ID: got %q, want %q", id, "file.bin")
	}
}

func TestGetFiles_LiteralPath_FileNotFound(t *testing.T) {
	u := &RawUploader{
		SrcPattern:   "/path/that/does/not/exist/file.bin",
		RegistryName: "reg",
	}

	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestGetFiles_LiteralPath_Directory_ReturnsError(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	u := &RawUploader{
		SrcPattern:   subdir,
		RegistryName: "reg",
	}

	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for directory source, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("expected 'not a regular file' in error, got: %v", err)
	}
}

// ── RawUploader.GetFiles – glob pattern ───────────────────────────────────────

func TestGetFiles_GlobPattern_MatchesMultipleFiles(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.bin": "aaa",
		"b.bin": "bbbb",
		"c.txt": "cc",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*.bin"),
		RegistryName: "reg",
	}

	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2", stats.FileCount)
	}
	expectedBytes := int64(len("aaa") + len("bbbb"))
	if stats.TotalBytes != expectedBytes {
		t.Errorf("TotalBytes: got %d, want %d", stats.TotalBytes, expectedBytes)
	}
}

func TestGetFiles_GlobPattern_NoMatch_ReturnsEmpty(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.txt": "data",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*.jar"),
		RegistryName: "reg",
	}

	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
	if stats.FileCount != 0 {
		t.Errorf("FileCount: got %d, want 0", stats.FileCount)
	}
}

func TestGetFiles_GlobPattern_Flatten_UsesBasenameOnly(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"sub/deep/file.bin": "data",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "**/*.bin"),
		RegistryName: "reg",
		DestTemplate: "releases",
		Flatten:      true,
	}

	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestGetFiles_GlobPattern_Recursive_MatchesNestedFiles(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a/b/c/file1.zip": "111",
		"a/b/file2.zip":   "2222",
		"file3.zip":       "33333",
		"not_matched.txt": "ignored",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "**/*.zip"),
		RegistryName: "reg",
	}

	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
	if stats.FileCount != 3 {
		t.Errorf("FileCount: got %d, want 3", stats.FileCount)
	}
}

func TestGetFiles_GlobPattern_InvalidPattern_ReturnsError(t *testing.T) {
	u := &RawUploader{
		SrcPattern:   ".",
		RegistryName: "reg",
		Include:      []string{"[invalid"},
	}

	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for invalid include pattern, got nil")
	}
}

func TestGetFiles_GlobPattern_IncludeFilter_AppliedAfterGlob(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.bin": "aaa",
		"b.txt": "bbb",
		"c.bin": "ccc",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*"),
		RegistryName: "reg",
		Include:      []string{"*.bin"},
	}

	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs after include filter, got %d", len(jobs))
	}
}

func TestGetFiles_GlobPattern_ExcludeFilter_RemovesMatches(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"release.zip": "rr",
		"debug.zip":   "dd",
		"readme.txt":  "tt",
	})

	u := &RawUploader{
		SrcPattern:   filepath.Join(root, "*"),
		RegistryName: "reg",
		Exclude:      []string{"*debug*"},
	}

	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, j := range jobs {
		if strings.Contains(j.GetID(), "debug") {
			t.Errorf("expected debug file to be excluded, but got job: %s", j.GetID())
		}
	}
}

// ── RawUploader.PreUpload ─────────────────────────────────────────────────────

func TestPreUpload_NoDryRun_NoConflicts_ReturnsFalseNil(t *testing.T) {
	u := &RawUploader{DryRun: false}
	skipped, err := u.PreUpload(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped {
		t.Error("expected skipped=false for non-dry-run with no conflicts")
	}
}

func TestPreUpload_DryRun_NoConflicts_ReturnsTrueNil(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	defer func() { _ = os.Chdir(origDir) }()

	u := &RawUploader{DryRun: true}
	skipped, err := u.PreUpload(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Error("expected skipped=true for dry-run with no conflicts")
	}
}
