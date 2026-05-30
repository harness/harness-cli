package command

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/common/upload"
)

// makeTree creates the given files under root and returns root. Empty values
// are treated as zero-length files; non-empty values are written verbatim.
// Path keys use forward slashes regardless of OS so test data is portable.
func makeTree(t *testing.T, files map[string]string) string {
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

// destPathsOf extracts the on-registry DestPath of each job and sorts them so
// tests can compare independently of walk order.
func destPathsOf(jobs []upload.FileUploadJob) []string {
	out := make([]string, 0, len(jobs))
	for _, j := range jobs {
		gj, ok := j.(*upload.GenericUploadJob)
		if !ok {
			continue
		}
		out = append(out, gj.DestPath)
	}
	sort.Strings(out)
	return out
}

// writeFile is a convenience helper for creating a single file inside a temp
// directory. Returns the full path.
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestCollectGenericUploadJobs_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "standalone.zip", "stub")

	jobs, stats, err := collectGenericUploadJobs([]string{file}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 1 {
		t.Errorf("expected 1 file, got %d", stats.fileCount)
	}

	want := []string{"web/1.0.0/standalone.zip"}
	if got := destPathsOf(jobs); !equal(got, want) {
		t.Errorf("dest paths mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestCollectGenericUploadJobs_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.zip", "a")
	b := writeFile(t, dir, "b.txt", "b")
	c := writeFile(t, dir, "c.json", "c")

	jobs, stats, err := collectGenericUploadJobs([]string{a, b, c}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 3 {
		t.Errorf("expected 3 files, got %d", stats.fileCount)
	}

	want := []string{
		"web/1.0.0/a.zip",
		"web/1.0.0/b.txt",
		"web/1.0.0/c.json",
	}
	if got := destPathsOf(jobs); !equal(got, want) {
		t.Errorf("dest paths mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestCollectGenericUploadJobs_DirectoryPreservesBasename(t *testing.T) {
	root := makeTree(t, map[string]string{
		"index.html":          "ok",
		"assets/css/main.css": "css",
	})

	jobs, stats, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "2.1.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 2 {
		t.Errorf("expected 2 files, got %d", stats.fileCount)
	}

	base := filepath.Base(root)
	want := []string{
		"web/2.1.0/" + base + "/assets/css/main.css",
		"web/2.1.0/" + base + "/index.html",
	}
	if got := destPathsOf(jobs); !equal(got, want) {
		t.Errorf("dest paths mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestCollectGenericUploadJobs_DeeplyNestedDir(t *testing.T) {
	root := makeTree(t, map[string]string{
		"index.html":                     "root",
		"assets/css/main.css":            "css",
		"assets/js/app.js":               "js",
		"assets/js/vendor/lib.js":        "vendor",
		"images/icons/sub/sub2/icon.png": "deep",
	})

	jobs, stats, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "2.1.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 5 {
		t.Errorf("expected 5 files, got %d", stats.fileCount)
	}

	base := filepath.Base(root)
	want := []string{
		"web/2.1.0/" + base + "/assets/css/main.css",
		"web/2.1.0/" + base + "/assets/js/app.js",
		"web/2.1.0/" + base + "/assets/js/vendor/lib.js",
		"web/2.1.0/" + base + "/images/icons/sub/sub2/icon.png",
		"web/2.1.0/" + base + "/index.html",
	}
	if got := destPathsOf(jobs); !equal(got, want) {
		t.Errorf("dest paths mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestCollectGenericUploadJobs_MixedFileAndDir(t *testing.T) {
	dir := t.TempDir()
	standalone := writeFile(t, dir, "standalone.zip", "stub")

	tree := makeTree(t, map[string]string{
		"index.html":          "ok",
		"assets/css/main.css": "css",
	})

	jobs, stats, err := collectGenericUploadJobs([]string{standalone, tree}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 3 {
		t.Errorf("expected 3 total files (1 file + 2 in dir), got %d", stats.fileCount)
	}

	base := filepath.Base(tree)
	want := []string{
		"web/1.0.0/" + base + "/assets/css/main.css",
		"web/1.0.0/" + base + "/index.html",
		"web/1.0.0/standalone.zip",
	}
	if got := destPathsOf(jobs); !equal(got, want) {
		t.Errorf("dest paths mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestCollectGenericUploadJobs_SkipsHiddenByDefault(t *testing.T) {
	root := makeTree(t, map[string]string{
		"visible.txt":         "ok",
		".dotfile":            "skip",
		"src/.DS_Store":       "skip",
		".cache/large.bin":    "skip",
		".cache/sub/deep.bin": "skip",
		"sub/normal.txt":      "ok",
	})

	jobs, stats, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "2.1.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 2 {
		t.Errorf("expected 2 visible files, got %d", stats.fileCount)
	}
	for _, p := range destPathsOf(jobs) {
		if strings.Contains(p, "/.") {
			t.Errorf("hidden entry leaked into dest paths: %s", p)
		}
	}
}

func TestCollectGenericUploadJobs_IncludesHiddenWhenFlagSet(t *testing.T) {
	root := makeTree(t, map[string]string{
		"visible.txt":      "ok",
		".dotfile":         "kept",
		".cache/inner.bin": "kept",
	})

	jobs, _, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "2.1.0", true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	base := filepath.Base(root)
	want := []string{
		"web/2.1.0/" + base + "/.cache/inner.bin",
		"web/2.1.0/" + base + "/.dotfile",
		"web/2.1.0/" + base + "/visible.txt",
	}
	if got := destPathsOf(jobs); !equal(got, want) {
		t.Errorf("dest paths mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestCollectGenericUploadJobs_EmptyDirectoryYieldsZeroJobs(t *testing.T) {
	root := t.TempDir()
	jobs, stats, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(jobs) != 0 || stats.fileCount != 0 {
		t.Errorf("expected zero jobs for empty dir, got %d (stats.fileCount=%d)", len(jobs), stats.fileCount)
	}
}

func TestCollectGenericUploadJobs_NonExistentPath(t *testing.T) {
	_, _, err := collectGenericUploadJobs([]string{"/this/path/does/not/exist"}, "myreg", "web", "1.0.0", false)
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
	if !strings.Contains(err.Error(), "access") && !strings.Contains(err.Error(), "no such") {
		t.Errorf("expected an access/not-found error, got %v", err)
	}
}

func TestCollectGenericUploadJobs_SkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := makeTree(t, map[string]string{
		"real.txt":        "real",
		"target/file.txt": "target",
	})
	if err := os.Symlink(filepath.Join(root, "target", "file.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	jobs, stats, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if stats.fileCount != 2 {
		t.Errorf("expected 2 regular files (symlink skipped), got %d", stats.fileCount)
	}
	for _, p := range destPathsOf(jobs) {
		if strings.HasSuffix(p, "/link.txt") {
			t.Errorf("symlink leaked into dest paths: %s", p)
		}
	}
}

func TestCollectGenericUploadJobs_PopulatesIDAndSize(t *testing.T) {
	root := makeTree(t, map[string]string{
		"foo.txt": "12345",
	})
	jobs, _, err := collectGenericUploadJobs([]string{root}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	gj := jobs[0].(*upload.GenericUploadJob)
	if gj.GetFileSize() != 5 {
		t.Errorf("expected file size 5, got %d", gj.GetFileSize())
	}

	base := filepath.Base(root)
	wantID := base + "/foo.txt"
	if gj.GetID() != wantID {
		t.Errorf("expected job ID %q, got %q", wantID, gj.GetID())
	}
	wantDest := "web/1.0.0/" + base + "/foo.txt"
	if gj.DestPath != wantDest {
		t.Errorf("expected dest %q, got %q", wantDest, gj.DestPath)
	}
}

func TestCollectGenericUploadJobs_FileAndDirCanShareLayout(t *testing.T) {
	// Sanity check that passing a file directly produces a different layout
	// than passing the file's parent directory — the file flattens to the
	// version root, the directory preserves its basename as a prefix.
	dir := t.TempDir()
	deep := filepath.Join(dir, "wrapper")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := writeFile(t, deep, "x.txt", "content")

	// File-only input.
	fileJobs, _, err := collectGenericUploadJobs([]string{file}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("file collect: %v", err)
	}
	// Directory input (containing the same file).
	dirJobs, _, err := collectGenericUploadJobs([]string{deep}, "myreg", "web", "1.0.0", false)
	if err != nil {
		t.Fatalf("dir collect: %v", err)
	}

	if got, want := destPathsOf(fileJobs), []string{"web/1.0.0/x.txt"}; !equal(got, want) {
		t.Errorf("file path mismatch: got %v, want %v", got, want)
	}
	if got, want := destPathsOf(dirJobs), []string{"web/1.0.0/wrapper/x.txt"}; !equal(got, want) {
		t.Errorf("dir path mismatch: got %v, want %v", got, want)
	}
}

// equal compares two string slices for elementwise equality.
func equal(a, b []string) bool {
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

// withGenericServer stands up a stub registry server, points config.Global at
// it, and restores all globals on cleanup. The handler runs for every request.
func withGenericServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	orig := config.Global
	config.Global.Registry.PkgURL = srv.URL
	config.Global.AccountID = "test-account"
	config.Global.AuthToken = "pat.test-account.aaa.bbb"
	t.Cleanup(func() { config.Global = orig })

	return srv
}

// runGenericCmd executes the generic push command with the given args and
// captures stdout into the returned buffer. Cobra's own out/err streams are
// also captured so test output stays clean.
func runGenericCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w

	doneCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		doneCh <- buf.String()
	}()

	cmd := NewPushGenericCmd(&cmdutils.Factory{})
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	runErr := cmd.Execute()

	_ = w.Close()
	os.Stdout = origStdout
	stdout := <-doneCh

	return stdout, runErr
}

func TestNewPushGenericCmd_Structure(t *testing.T) {
	cmd := NewPushGenericCmd(&cmdutils.Factory{})

	if !strings.HasPrefix(cmd.Use, "generic ") {
		t.Errorf("unexpected Use: %q", cmd.Use)
	}
	if cmd.Short == "" || cmd.Long == "" {
		t.Errorf("expected non-empty Short/Long, got %q / %q", cmd.Short, cmd.Long)
	}

	for _, name := range []string{"name", "version", "description", "pkg-url", "include-hidden"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s missing", name)
		}
	}

	nameFlag := cmd.Flags().Lookup("name")
	if nameFlag == nil {
		t.Fatal("--name flag missing")
	}
	if req, ok := nameFlag.Annotations[cobraBashCompOneRequiredFlag]; !ok || len(req) == 0 || req[0] != "true" {
		t.Errorf("--name should be marked required, got annotations=%v", nameFlag.Annotations)
	}

	if err := cmd.Args(cmd, []string{"only-registry"}); err == nil {
		t.Error("expected MinimumNArgs(2) to reject a single arg")
	}
}

// Cobra exposes the "required flag" annotation under this key.
const cobraBashCompOneRequiredFlag = "cobra_annotation_bash_completion_one_required_flag"

func TestNewPushGenericCmd_RequiresName(t *testing.T) {
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when --name is missing")
	})
	dir := t.TempDir()
	file := writeFile(t, dir, "blob.bin", "x")
	_, err := runGenericCmd(t, "myreg", file)
	if err == nil {
		t.Fatal("expected error for missing --name flag")
	}
}

func TestNewPushGenericCmd_FileSuccess(t *testing.T) {
	var hits int
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		hits++
		w.WriteHeader(http.StatusOK)
	})

	dir := t.TempDir()
	file := writeFile(t, dir, "blob.bin", "data")

	if _, err := runGenericCmd(t, "myreg", file, "--name", "web", "--version", "1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 PUT, got %d", hits)
	}
}

func TestNewPushGenericCmd_DirSuccess(t *testing.T) {
	var hits int
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	})

	root := makeTree(t, map[string]string{
		"index.html":          "ok",
		"assets/css/main.css": "css",
	})
	if _, err := runGenericCmd(t, "myreg", root, "--name", "site"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 2 {
		t.Errorf("expected 2 PUTs, got %d", hits)
	}
}

func TestNewPushGenericCmd_DefaultVersion(t *testing.T) {
	var sawDefault bool
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "1.0.0") || strings.Contains(r.URL.RawPath, "1.0.0") {
			sawDefault = true
		}
		w.WriteHeader(http.StatusOK)
	})

	dir := t.TempDir()
	file := writeFile(t, dir, "blob.bin", "data")
	if _, err := runGenericCmd(t, "myreg", file, "--name", "web"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawDefault {
		t.Error("expected request path to include the default 1.0.0 version")
	}
}

func TestNewPushGenericCmd_ServerError(t *testing.T) {
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	dir := t.TempDir()
	file := writeFile(t, dir, "blob.bin", "data")
	stdout, err := runGenericCmd(t, "myreg", file, "--name", "web", "--version", "1.0.0")
	if err == nil {
		t.Fatal("expected upload failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to upload") {
		t.Errorf("error should mention upload failure, got: %v", err)
	}
	if !strings.Contains(stdout, "Failed uploads:") {
		t.Errorf("expected failure list in stdout, got: %s", stdout)
	}
}

func TestNewPushGenericCmd_NoFilesAfterHiddenFilter(t *testing.T) {
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when no files are eligible")
	})

	// Directory contains only hidden entries; default (include-hidden=false)
	// means zero jobs.
	root := makeTree(t, map[string]string{
		".dotfile":        "skip",
		".cache/blob.bin": "skip",
	})
	_, err := runGenericCmd(t, "myreg", root, "--name", "web")
	if err == nil {
		t.Fatal("expected 'no files to upload' error")
	}
	if !strings.Contains(err.Error(), "no files to upload") {
		t.Errorf("expected 'no files to upload' error, got: %v", err)
	}
}

func TestNewPushGenericCmd_NonExistentPath(t *testing.T) {
	withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for missing path")
	})
	_, err := runGenericCmd(t, "myreg", "/this/path/does/not/exist", "--name", "web")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestNewPushGenericCmd_PkgUrlFlagAppliesToConfig(t *testing.T) {
	srv := withGenericServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use a value without scheme so GetPkgUrl's https:// prefixing path is
	// exercised. The first server is only here for cleanup symmetry; we
	// override PkgURL via the flag.
	hostPort := strings.TrimPrefix(srv.URL, "http://")

	dir := t.TempDir()
	file := writeFile(t, dir, "blob.bin", "data")
	// The pkg-url flag prepends "https://" when scheme is missing, which
	// will make the upload fail (httptest is http). We don't care about
	// the upload result here, only that the flag wiring runs without
	// panicking and that PreRun mutates the global config.
	stdout, _ := runGenericCmd(t, "myreg", file,
		"--name", "web", "--pkg-url", hostPort)
	if !strings.Contains(stdout, "deprecated") {
		t.Errorf("expected GetPkgUrl deprecation notice in stdout, got: %s", stdout)
	}
	if !strings.HasPrefix(config.Global.Registry.PkgURL, "https://") {
		t.Errorf("expected config.Global.Registry.PkgURL to be normalised to https://, got %q",
			config.Global.Registry.PkgURL)
	}
}

func TestPrintGenericUploadFailures(t *testing.T) {
	results := []upload.FileUploadResult{
		{JobID: "ok.bin", Success: true},
		{JobID: "broken-a.bin", Success: false, Error: io.ErrUnexpectedEOF},
		{JobID: "broken-b.bin", Success: false, Error: io.ErrClosedPipe},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	doneCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		doneCh <- buf.String()
	}()

	printGenericUploadFailures(results)

	_ = w.Close()
	os.Stdout = origStdout
	out := <-doneCh

	if !strings.Contains(out, "Failed uploads:") {
		t.Errorf("expected header line, got: %s", out)
	}
	if !strings.Contains(out, "broken-a.bin") || !strings.Contains(out, "broken-b.bin") {
		t.Errorf("expected both failed IDs in output, got: %s", out)
	}
	if strings.Contains(out, "ok.bin") {
		t.Errorf("successful jobs should not appear, got: %s", out)
	}
}

func TestCollectFromPath_IrregularFileRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes via syscall.Mkfifo are POSIX-only")
	}

	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	_, _, err := collectGenericUploadJobs([]string{fifo}, "myreg", "web", "1.0.0", false)
	if err == nil {
		t.Fatal("expected error for non-regular file (named pipe)")
	}
	if !strings.Contains(err.Error(), "not a regular file or directory") {
		t.Errorf("expected 'not a regular file or directory' error, got: %v", err)
	}
}
