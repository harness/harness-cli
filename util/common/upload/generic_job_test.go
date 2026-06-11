package upload

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

// withFakePkgServer spins up a stub package server, points config.Global at
// it, and restores all globals on cleanup. The handler receives the
// per-request hit count (1-indexed) so tests can encode different responses
// per attempt declaratively.
func withFakePkgServer(t *testing.T, handler func(hit int, w http.ResponseWriter, r *http.Request)) (server *httptest.Server, hits *int64) {
	t.Helper()

	var counter int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit := atomic.AddInt64(&counter, 1)
		handler(int(hit), w, r)
	}))
	t.Cleanup(srv.Close)

	orig := config.Global
	config.Global.Registry.PkgURL = srv.URL
	config.Global.AccountID = "test-account"
	config.Global.AuthToken = "pat.test-account.aaa.bbb"
	t.Cleanup(func() { config.Global = orig })

	return srv, &counter
}

func writeTempFile(t *testing.T, contents string) (string, int64) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p, int64(len(contents))
}

func createTestPkgClient(t *testing.T) *pkgclient.ClientWithResponses {
	t.Helper()
	client, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return client
}

func TestGenericUpload_Success(t *testing.T) {
	_, hits := withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	path, size := writeTempFile(t, "hello world")
	pkgClient := createTestPkgClient(t)
	job := NewGenericUploadJob("blob.bin", path, "pkg/v1/blob.bin", "myreg", "pkg", "v1", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("expected exactly 1 server hit, got %d", got)
	}
}

func TestGenericUpload_Success_201Created(t *testing.T) {
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestPkgClient(t)
	job := NewGenericUploadJob("blob.bin", path, "pkg/v1/blob.bin", "myreg", "pkg", "v1", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success on 201, got %v", err)
	}
}

func TestGenericUpload_FailsOn4xx(t *testing.T) {
	_, hits := withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestPkgClient(t)
	job := NewGenericUploadJob("blob.bin", path, "pkg/v1/blob.bin", "myreg", "pkg", "v1", size, utils.FileChecksums{}, pkgClient)

	err := job.Upload(context.Background())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("expected exactly 1 server hit, got %d", got)
	}
}

func TestGenericUpload_FailsOn5xx(t *testing.T) {
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestPkgClient(t)
	job := NewGenericUploadJob("blob.bin", path, "pkg/v1/blob.bin", "myreg", "pkg", "v1", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestGenericUpload_FailsOnMissingFile(t *testing.T) {
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be hit when file is missing")
	})

	pkgClient := createTestPkgClient(t)
	job := NewGenericUploadJob("ghost.bin", "/path/that/does/not/exist.bin",
		"pkg/v1/ghost.bin", "myreg", "pkg", "v1", 0, utils.FileChecksums{}, pkgClient)

	err := job.Upload(context.Background())
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open file") {
		t.Errorf("expected 'failed to open file' in error, got %v", err)
	}
}

func TestGenericUpload_RespectsContextCancel(t *testing.T) {
	stop := make(chan struct{})
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	})
	t.Cleanup(func() { close(stop) })

	path, size := writeTempFile(t, "data")
	pkgClient := createTestPkgClient(t)
	job := NewGenericUploadJob("blob.bin", path, "pkg/v1/blob.bin", "myreg", "pkg", "v1", size, utils.FileChecksums{}, pkgClient)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := job.Upload(ctx)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		// Transports may wrap cancellation in different errors; accept any
		// cancellation-flavoured failure as long as it's surfaced.
		if !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	}
}
