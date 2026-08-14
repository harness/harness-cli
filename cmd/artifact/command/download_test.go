package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/download"
	p "github.com/harness/harness-cli/util/common/progress"
)

func makeItems(paths ...string) []ar_v3.FileMetadata {
	items := make([]ar_v3.FileMetadata, 0, len(paths))
	dummyURL := "http://example.com/file"
	for _, path := range paths {
		p := path
		items = append(items, ar_v3.FileMetadata{Path: p, DownloadUrl: &dummyURL})
	}
	return items
}

func TestDetectFlattenConflicts_NoConflicts(t *testing.T) {
	items := makeItems(
		"/artifact/v1/file1.txt",
		"/artifact/v2/file2.txt",
		"/artifact/v3/file3.txt",
	)
	conflicts := detectFlattenConflicts(items, "/dest")
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestDetectFlattenConflicts_OneConflict(t *testing.T) {
	items := makeItems(
		"/artifact2/v1/img1.png",
		"/imageartifact/2.0/img1.png",
		"/imageartifact/2.0/img2.png",
	)
	conflicts := detectFlattenConflicts(items, "/dest")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].DestPath != filepath.Join("/dest", "img1.png") {
		t.Errorf("unexpected dest path: %s", conflicts[0].DestPath)
	}
	if len(conflicts[0].Jobs) != 2 {
		t.Errorf("expected 2 sources in conflict, got %d", len(conflicts[0].Jobs))
	}
}

func TestDetectFlattenConflicts_MultipleConflicts(t *testing.T) {
	items := makeItems(
		"/a/v1/img1.png",
		"/b/v1/img1.png",
		"/a/v1/data.csv",
		"/b/v1/data.csv",
		"/c/v1/unique.txt",
	)
	conflicts := detectFlattenConflicts(items, "/dest")
	if len(conflicts) != 2 {
		t.Errorf("expected 2 conflicts, got %d", len(conflicts))
	}
}

func TestDetectFlattenConflicts_EmptyItems(t *testing.T) {
	conflicts := detectFlattenConflicts([]ar_v3.FileMetadata{}, "/dest")
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts for empty input, got %d", len(conflicts))
	}
}

func TestDetectFlattenConflicts_AllSameBasename(t *testing.T) {
	items := makeItems(
		"/dir1/report.pdf",
		"/dir2/report.pdf",
		"/dir3/report.pdf",
	)
	conflicts := detectFlattenConflicts(items, "/dest")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if len(conflicts[0].Jobs) != 3 {
		t.Errorf("expected 3 sources in conflict, got %d", len(conflicts[0].Jobs))
	}
}

func TestWriteDryRunOutput_NoFlatten_WritesOutputFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	items := makeItems("/pkg/v1/file1.txt", "/pkg/v2/file2.txt")
	if err := writeDryRunOutput(items, "/dest", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	var outputFiles []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "download-dryrun-output-") {
			outputFiles = append(outputFiles, e.Name())
		}
	}
	if len(outputFiles) != 1 {
		t.Fatalf("expected 1 dry-run output file, got %d", len(outputFiles))
	}

	data, err := os.ReadFile(filepath.Join(tmp, "dry-run-output", outputFiles[0]))
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	var result []downloadDryRunEntry
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("output file is not valid JSON: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestWriteDryRunOutput_Flatten_WritesConflictFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	items := makeItems("/a/v1/img1.png", "/b/v1/img1.png", "/c/v1/unique.txt")
	if err := writeDryRunOutput(items, "/dest", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	var conflictFiles []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "conflict-download-") {
			conflictFiles = append(conflictFiles, e.Name())
		}
	}
	if len(conflictFiles) != 1 {
		t.Fatalf("expected 1 conflict file, got %d", len(conflictFiles))
	}

	data, err := os.ReadFile(filepath.Join(tmp, "dry-run-output", conflictFiles[0]))
	if err != nil {
		t.Fatalf("failed to read conflict file: %v", err)
	}
	var conflicts []downloadConflictEntry
	if err := json.Unmarshal(data, &conflicts); err != nil {
		t.Fatalf("conflict file is not valid JSON: %v", err)
	}
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict entry, got %d", len(conflicts))
	}
}

func TestWriteDryRunOutput_Flatten_NoConflict_NoConflictFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	items := makeItems("/a/v1/file1.txt", "/b/v1/file2.txt")
	if err := writeDryRunOutput(items, "/dest", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "conflict-download-") {
			t.Errorf("expected no conflict file, but found: %s", e.Name())
		}
	}
}

// TestNewDownloadByRegexCmd_DryRunFlatten_FiltersOutMissingURL pins the
// invariant that dry-run mirrors the real run: items missing a downloadUrl
// are filtered out before conflict detection AND before the dry-run manifest
// is written, so the report matches what --flatten would actually do.
func TestNewDownloadByRegexCmd_DryRunFlatten_FiltersOutMissingURL(t *testing.T) {
	url := "http://example.com/img1.png"
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/a/img1.png", DownloadUrl: &url}, // downloadable
			{Path: "/b/img1.png"},                    // no URL — would be dropped by real run
		}, false))
	}))
	defer searchSrv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir(), "--dry-run", "--flatten"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "conflict-download-") {
			t.Errorf("expected no conflict file (one item has no URL), but found: %s", e.Name())
		}
	}

	// Verify manifest excludes the item without a download URL.
	var manifestName string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "download-dryrun-output-") {
			manifestName = e.Name()
		}
	}
	if manifestName == "" {
		t.Fatal("expected a dry-run manifest, found none")
	}
	data, err := os.ReadFile(filepath.Join(tmp, "dry-run-output", manifestName))
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}
	var manifest []downloadDryRunEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest not valid JSON: %v", err)
	}
	if len(manifest) != 1 {
		t.Errorf("expected manifest to contain 1 entry (filtered), got %d", len(manifest))
	}
	if len(manifest) == 1 && manifest[0].SourcePath != "/a/img1.png" {
		t.Errorf("expected manifest entry for /a/img1.png, got %s", manifest[0].SourcePath)
	}
}

// TestNewDownloadByRegexCmd_DryRunSkipsPathTraversal verifies dry-run filters
// items whose registry paths escape destDir, matching the real download's
// containment check — so the manifest doesn't lie about what will happen.
func TestNewDownloadByRegexCmd_DryRunSkipsPathTraversal(t *testing.T) {
	safeURL := "http://example.com/safe.txt"
	evilURL := "http://example.com/evil.txt"
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/pkg/v1/safe.txt", DownloadUrl: &safeURL},
			{Path: "../../evil.txt", DownloadUrl: &evilURL},
		}, false))
	}))
	defer searchSrv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir(), "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	var manifestName string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "download-dryrun-output-") {
			manifestName = e.Name()
		}
	}
	if manifestName == "" {
		t.Fatal("expected a dry-run manifest, found none")
	}
	data, err := os.ReadFile(filepath.Join(tmp, "dry-run-output", manifestName))
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}
	var manifest []downloadDryRunEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest not valid JSON: %v", err)
	}
	if len(manifest) != 1 {
		t.Errorf("expected manifest to contain 1 entry (path-traversal filtered), got %d", len(manifest))
	}
	if len(manifest) == 1 && manifest[0].SourcePath != "/pkg/v1/safe.txt" {
		t.Errorf("expected manifest entry for /pkg/v1/safe.txt, got %s", manifest[0].SourcePath)
	}
}

// searchResponse builds a JSON search response body matching the generated SearchFilesResponse shape.
func searchResponse(items []ar_v3.FileMetadata, hasMore bool) []byte {
	resp := ar_v3.ListFilesResponse{Items: items, HasMore: hasMore}
	b, _ := json.Marshal(resp)
	return b
}

// setTestAPIBase points config.Global.APIBaseURL at the given server and returns a factory.
// The factory appends "/gateway/har/api/v3" when building the v3 client, so tests must
// serve requests at that path — or use a catch-all handler as the test servers here do.
func setTestAPIBase(t *testing.T, serverURL string) *cmdutils.Factory {
	t.Helper()
	orig := config.Global
	t.Cleanup(func() { config.Global = orig })
	config.Global.APIBaseURL = serverURL
	return cmdutils.NewFactory()
}

func TestNewDownloadByRegexCmd_NoMatchingFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{}, false))
	}))
	defer srv.Close()

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, srv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir()})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDownloadByRegexCmd_DryRun(t *testing.T) {
	url1 := "http://example.com/file1.txt"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/pkg/v1/file1.txt", DownloadUrl: &url1},
		}, false))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, srv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir(), "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDownloadByRegexCmd_FlattenConflictBlocks(t *testing.T) {
	url1, url2 := "http://example.com/a/img1.png", "http://example.com/b/img1.png"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/a/img1.png", DownloadUrl: &url1},
			{Path: "/b/img1.png", DownloadUrl: &url2},
		}, false))
	}))
	defer srv.Close()

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, srv.URL))
	cmd.SetArgs([]string{"myreg", ".*img", t.TempDir(), "--flatten"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for conflict, got nil")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestNewDownloadByRegexCmd_DownloadsFiles(t *testing.T) {
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("file content"))
	}))
	defer fileSrv.Close()

	downloadURL := fileSrv.URL + "/pkg/v1/file1.txt"
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/pkg/v1/file1.txt", DownloadUrl: &downloadURL},
		}, false))
	}))
	defer searchSrv.Close()

	destDir := t.TempDir()
	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", destDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	savedPath := filepath.Join(destDir, "pkg", "v1", "file1.txt")
	if _, err := os.Stat(savedPath); err != nil {
		t.Errorf("expected file at %s, got: %v", savedPath, err)
	}
}

func TestNewDownloadByRegexCmd_AllItemsMissingURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/pkg/v1/file1.txt"},
			{Path: "/pkg/v1/file2.txt"},
		}, false))
	}))
	defer srv.Close()

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, srv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir()})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when all items lack download URL, got nil")
	}
	if !strings.Contains(err.Error(), "no downloadable files") {
		t.Errorf("expected 'no downloadable files' error, got: %v", err)
	}
}

// TestNewDownloadByRegexCmd_PartiallySkipsMissingURLs covers the mixed case:
// some items have download URLs, others don't. Skips are soft — downloadable
// files succeed and the command exits 0; the skip is only reported in the summary.
func TestNewDownloadByRegexCmd_PartiallySkipsMissingURLs(t *testing.T) {
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer fileSrv.Close()

	goodURL := fileSrv.URL + "/pkg/v1/ok.txt"
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/pkg/v1/ok.txt", DownloadUrl: &goodURL},
			{Path: "/pkg/v1/missing.txt"},
		}, false))
	}))
	defer searchSrv.Close()

	destDir := t.TempDir()
	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", destDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected soft skip to exit 0, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "pkg", "v1", "ok.txt")); statErr != nil {
		t.Errorf("expected downloadable file to be fetched despite partial skip, got: %v", statErr)
	}
}

func TestNewDownloadByRegexCmd_SearchErrorWithJSONMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid regex"}}`))
	}))
	defer srv.Close()

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, srv.URL))
	cmd.SetArgs([]string{"myreg", "[bad", t.TempDir()})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error on search failure, got nil")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("expected server error message in output, got: %v", err)
	}
}

func TestNewDownloadByRegexCmd_FlattenDownloadsToBase(t *testing.T) {
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer fileSrv.Close()

	downloadURL := fileSrv.URL + "/deep/path/file.txt"
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/deep/path/file.txt", DownloadUrl: &downloadURL},
		}, false))
	}))
	defer searchSrv.Close()

	destDir := t.TempDir()
	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", destDir, "--flatten"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "file.txt")); err != nil {
		t.Errorf("expected flattened file, got: %v", err)
	}
}

func TestNewDownloadByRegexCmd_SearchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, srv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir()})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error on search failure, got nil")
	}
}

func TestNewDownloadByRegexCmd_ArgMismatch(t *testing.T) {
	cmd := NewDownloadByRegexCmd(&cmdutils.Factory{})
	cmd.SetArgs([]string{"myreg"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for wrong number of args, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid number of arguments") {
		t.Errorf("expected arg mismatch error, got: %v", err)
	}
}

// TestNewDownloadByRegexCmd_InvalidPageSize verifies out-of-bounds --page-size
// is rejected up front with a clear usage error instead of letting the server
// respond with an opaque HTTP 400.
func TestNewDownloadByRegexCmd_InvalidPageSize(t *testing.T) {
	cases := []struct {
		name string
		size string
	}{
		{"too high", "500"},
		{"zero", "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := NewDownloadByRegexCmd(&cmdutils.Factory{})
			cmd.SetArgs([]string{"myreg", ".*", t.TempDir(), "--page-size", c.size})
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for invalid --page-size, got nil")
			}
			if !strings.Contains(err.Error(), "must be between 1 and 100") {
				t.Errorf("expected bound error, got: %v", err)
			}
		})
	}
}

// TestNewDownloadByRegexCmd_PartialDownloadFailure covers the branch where
// some downloads succeed and others fail, exercising HasDownloadErrors and
// printDownloadFailures inside RunE.
func TestNewDownloadByRegexCmd_PartialDownloadFailure(t *testing.T) {
	// good file server returns 200
	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer goodSrv.Close()

	// bad file server always returns 500
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	goodURL := goodSrv.URL + "/pkg/ok.txt"
	badURL := badSrv.URL + "/pkg/fail.txt"
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
			{Path: "/pkg/ok.txt", DownloadUrl: &goodURL},
			{Path: "/pkg/fail.txt", DownloadUrl: &badURL},
		}, false))
	}))
	defer searchSrv.Close()

	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir()})
	// Command should exit cleanly — the failure count shows up in the
	// summary block, not as a returned error.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestNewDownloadByRegexCmd_SearchNetworkError covers the branch where the
// search HTTP request fails at the transport level (server closes immediately).
func TestNewDownloadByRegexCmd_SearchNetworkError(t *testing.T) {
	// start server then immediately close it so the request fails at connect time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srvURL := srv.URL
	srv.Close()

	orig := config.Global
	t.Cleanup(func() { config.Global = orig })
	config.Global.APIBaseURL = srvURL
	config.Global.TimeoutSeconds = 1

	cmd := NewDownloadByRegexCmd(cmdutils.NewFactory())
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir()})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error on network failure, got nil")
	}
	if !strings.Contains(err.Error(), "search request failed") {
		t.Errorf("expected search request failure, got: %v", err)
	}
}

func TestNewDownloadByRegexCmd_OrgProjectScope(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{}, false))
	}))
	defer srv.Close()

	f := setTestAPIBase(t, srv.URL)
	config.Global.OrgID = "my-org"
	config.Global.ProjectID = "my-project"

	cmd := NewDownloadByRegexCmd(f)
	cmd.SetArgs([]string{"myreg", ".*", t.TempDir()})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := gotQuery["org_identifier"]; len(got) == 0 || got[0] != "my-org" {
		t.Errorf("expected org_identifier=my-org in query, got %v", got)
	}
	if got := gotQuery["project_identifier"]; len(got) == 0 || got[0] != "my-project" {
		t.Errorf("expected project_identifier=my-project in query, got %v", got)
	}
}

func TestPrintDownloadFailures_PrintsFailedJobs(t *testing.T) {
	results := []download.FileDownloadResult{
		{JobID: "/pkg/v1/ok.txt", Success: true},
		{JobID: "/pkg/v1/fail.txt", Success: false, Error: fmt.Errorf("connection refused")},
	}
	// just verify it doesn't panic
	printDownloadFailures(results, p.NewConsoleReporter())
}

func TestPrintDownloadSummary(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	originalStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = originalStdout })

	printDownloadSummary(10, 7, 2, 1, 3, "/dest")
	_ = writer.Close()
	os.Stdout = originalStdout

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	for _, expected := range []string{
		"=== Download Execution Summary ===",
		"Total files matched: 10",
		"Successfully downloaded: 7",
		"Failed: 2",
		"Skipped (no download URL): 1",
		"Skipped (unsafe path — escapes destination): 3",
		"Destination: /dest",
	} {
		if !strings.Contains(string(output), expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestWriteDryRunOutput_Flatten_WithConflicts_WritesConflictFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// two files with same basename — conflict under flatten
	items := makeItems("/a/v1/img1.png", "/b/v1/img1.png", "/c/v1/unique.txt")
	if err := writeDryRunOutput(items, "/dest", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	var conflictFiles []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "conflict-download-") {
			conflictFiles = append(conflictFiles, e.Name())
		}
	}
	if len(conflictFiles) != 1 {
		t.Fatalf("expected 1 conflict file, got %d", len(conflictFiles))
	}
	data, err := os.ReadFile(filepath.Join(tmp, "dry-run-output", conflictFiles[0]))
	if err != nil {
		t.Fatalf("failed to read conflict file: %v", err)
	}
	var conflicts []downloadConflictEntry
	if err := json.Unmarshal(data, &conflicts); err != nil {
		t.Fatalf("conflict file is not valid JSON: %v", err)
	}
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts))
	}
	if len(conflicts[0].Jobs) != 2 {
		t.Errorf("expected 2 sources in conflict, got %d", len(conflicts[0].Jobs))
	}
}

func TestWriteDryRunOutput_NoFlatten_NoConflictFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// same basename but no flatten — should never produce a conflict file
	items := makeItems("/a/v1/img1.png", "/b/v1/img1.png")
	if err := writeDryRunOutput(items, "/dest", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(tmp, "dry-run-output"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "conflict-download-") {
			t.Errorf("expected no conflict file without --flatten, but found: %s", e.Name())
		}
	}
}

func TestNewDownloadByRegexCmd_Pagination(t *testing.T) {
	page := 0
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer fileSrv.Close()

	url1 := fileSrv.URL + "/a.txt"
	url2 := fileSrv.URL + "/b.txt"

	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if page == 0 {
			page++
			_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
				{Path: "/a.txt", DownloadUrl: &url1},
			}, true))
		} else {
			_, _ = w.Write(searchResponse([]ar_v3.FileMetadata{
				{Path: "/b.txt", DownloadUrl: &url2},
			}, false))
		}
	}))
	defer searchSrv.Close()

	destDir := t.TempDir()
	cmd := NewDownloadByRegexCmd(setTestAPIBase(t, searchSrv.URL))
	cmd.SetArgs([]string{"myreg", ".*", destDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
