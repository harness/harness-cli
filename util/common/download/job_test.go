package download

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/config"
)

func TestIsWithinDest(t *testing.T) {
	destDir := t.TempDir()
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"simple", "foo.txt", true},
		{"nested", "a/b/c.txt", true},
		{"parent escape", "../evil.txt", false},
		{"deep parent escape", "../../evil.txt", false},
		// Absolute paths get joined under destDir by filepath.Join, so they
		// resolve inside destDir and are considered safe.
		{"absolute path folded under dest", "/etc/passwd", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsWithinDest(destDir, c.path); got != c.want {
				t.Errorf("IsWithinDest(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

// withFakeServer spins up a stub HTTP server and tears it down after the test.
func withFakeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestURLDownloadJob_Success(t *testing.T) {
	content := []byte("file contents")
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob(
		"/mypkg/1.0.0/file.txt",
		"/mypkg/1.0.0/file.txt",
		destDir,
		srv.URL+"/mypkg/1.0.0/file.txt",
		0,
	)

	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	savedPath := filepath.Join(destDir, "mypkg", "1.0.0", "file.txt")
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("expected file at %s, got error: %v", savedPath, err)
	}
	if string(data) != string(content) {
		t.Errorf("file contents mismatch: got %q, want %q", data, content)
	}
}

func TestURLDownloadJob_CreatesNestedDirectories(t *testing.T) {
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob(
		"/a/b/c/deep.txt",
		"/a/b/c/deep.txt",
		destDir,
		srv.URL+"/a/b/c/deep.txt",
		0,
	)

	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	savedPath := filepath.Join(destDir, "a", "b", "c", "deep.txt")
	if _, err := os.Stat(savedPath); err != nil {
		t.Errorf("expected file at %s to exist, got: %v", savedPath, err)
	}
}

func TestURLDownloadJob_FailsOn4xx(t *testing.T) {
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob("/pkg/v1/missing.txt", "/pkg/v1/missing.txt", destDir, srv.URL+"/pkg/v1/missing.txt", 0)

	err := job.Download(context.Background())
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("expected 404 mention in error, got: %v", err)
	}
}

func TestURLDownloadJob_FailsOn5xx(t *testing.T) {
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob("/pkg/v1/blob.bin", "/pkg/v1/blob.bin", destDir, srv.URL+"/pkg/v1/blob.bin", 0)

	if err := job.Download(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestURLDownloadJob_RespectsContextCancel(t *testing.T) {
	stop := make(chan struct{})
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	})
	t.Cleanup(func() { close(stop) })

	destDir := t.TempDir()
	job := NewURLDownloadJob("/pkg/v1/blob.bin", "/pkg/v1/blob.bin", destDir, srv.URL+"/pkg/v1/blob.bin", 0)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := job.Download(ctx)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestURLDownloadJob_GettersReturnExpectedValues(t *testing.T) {
	job := NewURLDownloadJob("id-key", "/mypkg/1.0.0/file.txt", "/dest", "http://example.com/file.txt", 1024)

	if job.GetID() != "id-key" {
		t.Errorf("GetID: got %q, want %q", job.GetID(), "id-key")
	}
	if job.GetFileSize() != 1024 {
		t.Errorf("GetFileSize: got %d, want 1024", job.GetFileSize())
	}
	wantPath := filepath.Join("/dest", "mypkg", "1.0.0", "file.txt")
	if job.GetFilePath() != wantPath {
		t.Errorf("GetFilePath: got %q, want %q", job.GetFilePath(), wantPath)
	}
}

func TestIsSameHostAsAPI(t *testing.T) {
	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.APIBaseURL = "https://app.harness.io"

	cases := []struct {
		name        string
		downloadURL string
		want        bool
	}{
		{"same host", "https://app.harness.io/foo/bar", true},
		{"different host", "https://s3.amazonaws.com/bucket/file", false},
		{"empty url", "", false},
		{"malformed url", "://not-a-url", false},
		{"case insensitive", "https://APP.HARNESS.IO/foo", true},
		{"scheme downgrade blocked", "http://app.harness.io/foo", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isSameHostAsAPI(c.downloadURL); got != c.want {
				t.Errorf("isSameHostAsAPI(%q) = %v, want %v", c.downloadURL, got, c.want)
			}
		})
	}
}

func TestURLDownloadJob_AttachesXApiKeyOnMatchingHost(t *testing.T) {
	var gotHeader string
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	})

	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.APIBaseURL = srv.URL
	config.Global.AuthToken = "pat.acc.aaa.bbb"

	destDir := t.TempDir()
	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, srv.URL+"/a.txt", 0)
	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotHeader != "pat.acc.aaa.bbb" {
		t.Errorf("expected x-api-key to be set, got %q", gotHeader)
	}
}

func TestURLDownloadJob_AttachesAuthorizationForCIManager(t *testing.T) {
	var gotAuth string
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.APIBaseURL = srv.URL
	config.Global.AuthToken = "CIManager token-abc"

	destDir := t.TempDir()
	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, srv.URL+"/a.txt", 0)
	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "CIManager token-abc" {
		t.Errorf("expected Authorization header to be set, got %q", gotAuth)
	}
}

func TestURLDownloadJob_SkipsAuthOnDifferentHost(t *testing.T) {
	var gotXApiKey, gotAuth string
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotXApiKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.APIBaseURL = "https://different.example.com"
	config.Global.AuthToken = "pat.acc.aaa.bbb"

	destDir := t.TempDir()
	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, srv.URL+"/a.txt", 0)
	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotXApiKey != "" || gotAuth != "" {
		t.Errorf("expected no auth headers on different host, got x-api-key=%q auth=%q", gotXApiKey, gotAuth)
	}
}

// TestURLDownloadJob_TimesOutOnUnresponsiveHost verifies that a host which
// never sends response headers doesn't wedge the worker forever — the
// transport's ResponseHeaderTimeout should fire.
func TestURLDownloadJob_TimesOutOnUnresponsiveHost(t *testing.T) {
	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.TimeoutSeconds = 1
	resetSharedClientForTest()
	t.Cleanup(resetSharedClientForTest)

	// server accepts the connection but never writes headers
	stop := make(chan struct{})
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-stop
	})
	t.Cleanup(func() { close(stop) })

	destDir := t.TempDir()
	job := NewURLDownloadJob("/slow.txt", "/slow.txt", destDir, srv.URL+"/slow.txt", 0)

	start := time.Now()
	err := job.Download(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("expected timeout within a few seconds, took %v", elapsed)
	}
}

// TestAuthAwareHTTPClient_FallsBackWhenTimeoutUnset verifies the fallback
// header-wait bound kicks in when config.Global.TimeoutSeconds is 0. Without
// this fallback, ResponseHeaderTimeout would be 0 (no timeout).
func TestAuthAwareHTTPClient_FallsBackWhenTimeoutUnset(t *testing.T) {
	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.TimeoutSeconds = 0

	client := authAwareHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.ResponseHeaderTimeout <= 0 {
		t.Errorf("expected non-zero fallback timeout when TimeoutSeconds=0, got %v",
			transport.ResponseHeaderTimeout)
	}
}

// TestAuthAwareHTTPClient_UsesConfiguredTimeout verifies that a positive
// config.Global.TimeoutSeconds is honored verbatim.
func TestAuthAwareHTTPClient_UsesConfiguredTimeout(t *testing.T) {
	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.TimeoutSeconds = 5

	client := authAwareHTTPClient()
	transport := client.Transport.(*http.Transport)
	if transport.ResponseHeaderTimeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", transport.ResponseHeaderTimeout)
	}
}

// TestURLDownloadJob_StripsAuthHeaderOnCrossHostRedirect verifies that when
// a download URL 302s to a different host (e.g. presigned S3/GCS), the
// x-api-key / Authorization headers are stripped before the redirected
// request is sent, preventing credential leaks to third-party storage.
func TestURLDownloadJob_StripsAuthHeaderOnCrossHostRedirect(t *testing.T) {
	var storageXApiKey, storageAuth string
	// second server plays the role of the redirected-to storage backend
	storageSrv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		storageXApiKey = r.Header.Get("x-api-key")
		storageAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("blob"))
	})

	// first server acts as the API host, returns 302 to the storage host
	apiSrv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, storageSrv.URL+"/blob", http.StatusFound)
	})

	origConfig := config.Global
	t.Cleanup(func() { config.Global = origConfig })
	config.Global.APIBaseURL = apiSrv.URL
	config.Global.AuthToken = "pat.acc.aaa.bbb"

	destDir := t.TempDir()
	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, apiSrv.URL+"/a.txt", 0)
	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storageXApiKey != "" || storageAuth != "" {
		t.Errorf("expected auth headers stripped on cross-host redirect, got x-api-key=%q auth=%q",
			storageXApiKey, storageAuth)
	}
}

func TestURLDownloadJob_RemovesPartialOnCopyError(t *testing.T) {
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		// write only some bytes, then hijack the connection to force a truncated read
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		_, _ = conn.Write([]byte("partial"))
		_ = conn.Close()
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, srv.URL+"/a.txt", 0)
	err := job.Download(context.Background())
	if err == nil {
		t.Fatal("expected error on truncated download, got nil")
	}
	savedPath := filepath.Join(destDir, "a.txt")
	if _, statErr := os.Stat(savedPath); !os.IsNotExist(statErr) {
		t.Errorf("expected partial file to be removed, but it still exists: %v", statErr)
	}
}

func TestURLDownloadJob_FailsOnInvalidURL(t *testing.T) {
	destDir := t.TempDir()
	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, "://not-a-url", 0)
	if err := job.Download(context.Background()); err == nil {
		t.Fatal("expected error on invalid URL, got nil")
	}
}

func TestURLDownloadJob_FailsWhenCreateFileFails(t *testing.T) {
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	})

	destDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(destDir, "a.txt"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	job := NewURLDownloadJob("/a.txt", "/a.txt", destDir, srv.URL+"/a.txt", 0)
	err := job.Download(context.Background())
	if err == nil {
		t.Fatal("expected error when dest is a directory, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create file") {
		t.Errorf("expected create file error, got: %v", err)
	}
}

func TestURLDownloadJob_RejectsPathTraversal(t *testing.T) {
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("malicious"))
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob(
		"../../evil.txt",
		"../../evil.txt",
		destDir,
		srv.URL+"/evil.txt",
		0,
	)

	err := job.Download(context.Background())
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to write outside destination dir") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
}

func TestURLDownloadJob_PreservesFileContents(t *testing.T) {
	// Verify binary content is preserved exactly (not corrupted during streaming)
	content := []byte{0x00, 0xFF, 0x42, 0x89, 0x50, 0x4E, 0x47} // mix of binary bytes
	srv := withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	})

	destDir := t.TempDir()
	job := NewURLDownloadJob("/pkg/v1/binary.bin", "/pkg/v1/binary.bin", destDir, srv.URL+"/pkg/v1/binary.bin", 0)

	if err := job.Download(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "pkg", "v1", "binary.bin"))
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("binary contents corrupted: got %v, want %v", data, content)
	}
}
