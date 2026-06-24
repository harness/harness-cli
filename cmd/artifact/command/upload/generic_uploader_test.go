package upload

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	commonupload "github.com/harness/harness-cli/util/common/upload"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// makeUploadTree creates the given files under a temp dir and returns the root.
// Keys use forward slashes so test data is OS-portable.
func makeUploadTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

// writeTempFile creates a single named file inside dir and returns its path.
func writeTempFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// destPaths extracts the DestPath from each GenericUploadJob and returns them sorted.
func destPaths(t *testing.T, jobs []commonupload.FileUploadJob) []string {
	t.Helper()
	out := make([]string, 0, len(jobs))
	for _, j := range jobs {
		gj, ok := j.(*commonupload.GenericUploadJob)
		if !ok {
			t.Fatalf("expected *GenericUploadJob, got %T", j)
		}
		out = append(out, gj.DestPath)
	}
	sort.Strings(out)
	return out
}

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

// ── splitPatternRoot ─────────────────────────────────────────────────────────

func TestSplitPatternRoot(t *testing.T) {
	tests := []struct {
		pattern    string
		wantRoot   string
		wantRelPat string
	}{
		{"*.jar", ".", "*.jar"},
		{"**/*.jar", ".", "**/*.jar"},
		{"dist/(*)/*.zip", "dist", "(*)/*.zip"},
		{"target/(**)", "target", "(**)"},
		{"/abs/path/file.txt", "/abs/path/file.txt", ""},
		{"a/b/c/*.go", filepath.Join("a", "b", "c"), "*.go"},
		{"(*)/*.txt", ".", "(*)/*.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			gotRoot, gotRel := splitPatternRoot(tc.pattern)
			if gotRoot != tc.wantRoot {
				t.Errorf("root: got %q, want %q", gotRoot, tc.wantRoot)
			}
			if gotRel != tc.wantRelPat {
				t.Errorf("relPattern: got %q, want %q", gotRel, tc.wantRelPat)
			}
		})
	}
}

// ── compileWildcardPattern ────────────────────────────────────────────────────

func TestCompileWildcardPattern_SingleStar(t *testing.T) {
	re, _, err := compileWildcardPattern("*.jar")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !re.MatchString("app.jar") {
		t.Error("expected *.jar to match app.jar")
	}
	if re.MatchString("subdir/app.jar") {
		t.Error("expected *.jar NOT to match across slashes")
	}
}

func TestCompileWildcardPattern_DoubleStar(t *testing.T) {
	re, _, err := compileWildcardPattern("**/*.jar")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, path := range []string{"a/b.jar", "a/b/c.jar", "a/b/c/d.jar"} {
		if !re.MatchString(path) {
			t.Errorf("expected **/*.jar to match %q", path)
		}
	}
}

func TestCompileWildcardPattern_CaptureGroups(t *testing.T) {
	re, groupCount, err := compileWildcardPattern("(*)/(*).zip")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if groupCount != 2 {
		t.Errorf("expected 2 capture groups, got %d", groupCount)
	}

	m := re.FindStringSubmatch("linux/app.zip")
	if m == nil {
		t.Fatal("expected match for linux/app.zip")
	}
	if m[1] != "linux" || m[2] != "app" {
		t.Errorf("captures: got [%q, %q], want [linux, app]", m[1], m[2])
	}
}

func TestCompileWildcardPattern_DoubleStarCapture(t *testing.T) {
	re, groupCount, err := compileWildcardPattern("(**)")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if groupCount != 1 {
		t.Errorf("expected 1 capture group, got %d", groupCount)
	}
	m := re.FindStringSubmatch("a/b/c/file.txt")
	if m == nil {
		t.Fatal("expected match")
	}
	if m[1] != "a/b/c/file.txt" {
		t.Errorf("capture: got %q, want a/b/c/file.txt", m[1])
	}
}

func TestCompileWildcardPattern_QuestionMark(t *testing.T) {
	re, _, err := compileWildcardPattern("fil?.txt")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !re.MatchString("file.txt") || !re.MatchString("filx.txt") {
		t.Error("? should match any single char")
	}
	if re.MatchString("fi/e.txt") {
		t.Error("? should not match slash")
	}
}

// ── resolveDestPath ───────────────────────────────────────────────────────────

func TestResolveDestPath(t *testing.T) {
	tests := []struct {
		name     string
		template string
		version  string
		captures []string
		basename string
		want     string
	}{
		{
			name:     "no captures, trailing slash",
			template: "pkg/",
			version:  "1.0.0",
			captures: nil,
			basename: "app.jar",
			want:     "pkg/1.0.0/app.jar",
		},
		{
			name:     "no captures, no slash",
			template: "mypackage",
			version:  "2.0.0",
			captures: nil,
			basename: "file.zip",
			want:     "mypackage/2.0.0/file.zip",
		},
		{
			name:     "single capture in path",
			template: "releases/{1}",
			version:  "1.0.0",
			captures: []string{"linux"},
			basename: "app.zip",
			want:     "releases/linux/1.0.0/app.zip",
		},
		{
			name:     "two captures",
			template: "builds/{1}/{2}",
			version:  "3.0.0",
			captures: []string{"prod", "v3"},
			basename: "out.tar.gz",
			want:     "builds/prod/v3/3.0.0/out.tar.gz",
		},
		{
			name:     "custom version",
			template: "libs",
			version:  "5.2.1",
			captures: nil,
			basename: "sdk.jar",
			want:     "libs/5.2.1/sdk.jar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDestPath(tc.template, tc.version, tc.captures, tc.basename)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── GetRegistryAndPath ────────────────────────────────────────────────────────

func TestGetRegistryAndPath_Valid(t *testing.T) {
	u := &GenericUploader{}
	reg, err := u.GetRegistryAndPath("my-registry/some/pkg/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg != "my-registry" {
		t.Errorf("registryName: got %q, want my-registry", reg)
	}
	if u.RegistryName != "my-registry" {
		t.Errorf("u.RegistryName: got %q, want my-registry", u.RegistryName)
	}
	if u.DestTemplate != "some/pkg/path" {
		t.Errorf("u.DestTemplate: got %q, want some/pkg/path", u.DestTemplate)
	}
}

func TestGetRegistryAndPath_NoSlash(t *testing.T) {
	u := &GenericUploader{}
	_, err := u.GetRegistryAndPath("no-slash-here")
	if err == nil {
		t.Fatal("expected error for target without '/'")
	}
	if !strings.Contains(err.Error(), "<registry>/<path>") {
		t.Errorf("error should describe expected format, got: %v", err)
	}
}

func TestGetRegistryAndPath_SetsStateForGetFiles(t *testing.T) {
	root := makeUploadTree(t, map[string]string{"file.txt": "data"})
	u := &GenericUploader{
		SrcPattern: filepath.Join(root, "*.txt"),
		Version:    "1.0.0",
	}

	reg, err := u.GetRegistryAndPath("test-reg/docs")
	if err != nil {
		t.Fatalf("GetRegistryAndPath: %v", err)
	}
	if reg != "test-reg" {
		t.Errorf("registry: got %q, want test-reg", reg)
	}

	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	gj := jobs[0].(*commonupload.GenericUploadJob)
	if gj.RegistryName != "test-reg" {
		t.Errorf("job.RegistryName: got %q, want test-reg", gj.RegistryName)
	}
}

// ── GetFiles – literal path ───────────────────────────────────────────────────

func TestGetFiles_LiteralFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "pkg.zip", "content")

	u := &GenericUploader{
		SrcPattern:   path,
		DestTemplate: "myrepo",
		RegistryName: "reg",
		Version:      "1.0.0",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 1 {
		t.Errorf("FileCount: got %d, want 1", stats.FileCount)
	}
	if stats.TotalBytes != int64(len("content")) {
		t.Errorf("TotalBytes: got %d, want %d", stats.TotalBytes, len("content"))
	}

	got := destPaths(t, jobs)
	want := []string{"myrepo/1.0.0/pkg.zip"}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestGetFiles_LiteralFile_NotRegular(t *testing.T) {
	dir := t.TempDir()
	u := &GenericUploader{
		SrcPattern:   dir,
		DestTemplate: "repo",
		RegistryName: "reg",
		Version:      "1.0.0",
	}
	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for directory as literal path")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error should say 'not a regular file', got: %v", err)
	}
}

func TestGetFiles_LiteralFile_NonExistent(t *testing.T) {
	u := &GenericUploader{
		SrcPattern:   "/no/such/file.zip",
		DestTemplate: "repo",
		RegistryName: "reg",
		Version:      "1.0.0",
	}
	_, _, err := u.GetFiles()
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// ── GetFiles – wildcard patterns ─────────────────────────────────────────────

func TestGetFiles_SingleStarPattern(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.zip":     "zip",
		"b.zip":     "zip",
		"c.txt":     "txt",
		"sub/d.zip": "zip",
	})

	u := &GenericUploader{
		SrcPattern:   filepath.Join(root, "*.zip"),
		DestTemplate: "pkg",
		RegistryName: "reg",
		Version:      "1.0.0",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2 (single star should not cross dir boundary)", stats.FileCount)
	}

	got := destPaths(t, jobs)
	want := []string{"pkg/1.0.0/a.zip", "pkg/1.0.0/b.zip"}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestGetFiles_DoubleStarPattern(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.jar":       "data",
		"sub/b.jar":   "data",
		"sub/c/d.jar": "data",
		"sub/c/e.txt": "data",
	})

	u := &GenericUploader{
		SrcPattern:   filepath.Join(root, "**/*.jar"),
		DestTemplate: "libs",
		RegistryName: "reg",
		Version:      "2.0.0",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 3 {
		t.Errorf("FileCount: got %d, want 3", stats.FileCount)
	}

	got := destPaths(t, jobs)
	want := []string{
		"libs/2.0.0/a.jar",
		"libs/2.0.0/b.jar",
		"libs/2.0.0/d.jar",
	}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestGetFiles_CaptureGroups_OneLevel(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"linux/app.zip":   "data",
		"darwin/app.zip":  "data",
		"windows/app.txt": "data",
	})

	u := &GenericUploader{
		SrcPattern:   filepath.Join(root, "(*)/app.zip"),
		DestTemplate: "releases/{1}",
		RegistryName: "reg",
		Version:      "1.0.0",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2", stats.FileCount)
	}

	got := destPaths(t, jobs)
	want := []string{
		"releases/darwin/1.0.0/app.zip",
		"releases/linux/1.0.0/app.zip",
	}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestGetFiles_CaptureGroups_TwoLevels(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"mydir/mypkg.nupkg": "data",
		"other/anoth.nupkg": "data",
	})

	u := &GenericUploader{
		SrcPattern:   filepath.Join(root, "(*)/(*).nupkg"),
		DestTemplate: "singletest/{1}/{2}.nupkg",
		RegistryName: "reg",
		Version:      "1.0.0",
	}
	jobs, stats, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2", stats.FileCount)
	}

	got := destPaths(t, jobs)
	want := []string{
		"singletest/mydir/mypkg.nupkg/1.0.0/mypkg.nupkg",
		"singletest/other/anoth.nupkg/1.0.0/anoth.nupkg",
	}
	if !slicesEqual(got, want) {
		t.Errorf("dest paths: got %v, want %v", got, want)
	}
}

func TestGetFiles_NoMatch(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"readme.txt": "ok",
	})

	u := &GenericUploader{
		SrcPattern:   filepath.Join(root, "*.jar"),
		DestTemplate: "libs",
		RegistryName: "reg",
		Version:      "1.0.0",
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

func TestGetFiles_DefaultVersion(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "artifact.zip", "x")

	u := &GenericUploader{
		SrcPattern:   path,
		DestTemplate: "pkg",
		RegistryName: "reg",
		Version:      "", // empty → should default to "1.0.0"
	}
	jobs, _, err := u.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	gj := jobs[0].(*commonupload.GenericUploadJob)
	if !strings.Contains(gj.DestPath, "/1.0.0/") {
		t.Errorf("expected default version 1.0.0 in dest path, got %q", gj.DestPath)
	}
}

func TestGetFiles_Stats_TotalBytes(t *testing.T) {
	root := makeUploadTree(t, map[string]string{
		"a.bin": "12345",   // 5 bytes
		"b.bin": "1234567", // 7 bytes
	})

	u := &GenericUploader{
		SrcPattern:   filepath.Join(root, "*.bin"),
		DestTemplate: "pkg",
		RegistryName: "reg",
		Version:      "1.0.0",
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
