package upload

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

func createTestRawPkgClient(t *testing.T) *pkgclient.ClientWithResponses {
	t.Helper()
	client, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return client
}

func TestRawUpload_Success(t *testing.T) {
	_, hits := withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	path, size := writeTempFile(t, "hello world")
	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("blob.bin", path, "subdir/blob.bin", "myreg", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("expected exactly 1 server hit, got %d", got)
	}
}

func TestRawUpload_Success_201Created(t *testing.T) {
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("blob.bin", path, "blob.bin", "myreg", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success on 201, got %v", err)
	}
}

func TestRawUpload_DestPathHasNoVersion(t *testing.T) {
	var capturedPath string
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("file.txt", path, "uploads/file.txt", "myreg", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if strings.Count(capturedPath, "/") < 4 {
		t.Errorf("expected registry path segments in URL, got %q", capturedPath)
	}
}

func TestRawUpload_FailsOn4xx(t *testing.T) {
	_, hits := withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("blob.bin", path, "blob.bin", "myreg", size, utils.FileChecksums{}, pkgClient)

	err := job.Upload(context.Background())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("expected exactly 1 server hit, got %d", got)
	}
}

func TestRawUpload_FailsOn5xx(t *testing.T) {
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	path, size := writeTempFile(t, "data")
	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("blob.bin", path, "blob.bin", "myreg", size, utils.FileChecksums{}, pkgClient)

	if err := job.Upload(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestRawUpload_FailsOnMissingFile(t *testing.T) {
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be hit when file is missing")
	})

	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("ghost.bin", "/path/that/does/not/exist.bin",
		"ghost.bin", "myreg", 0, utils.FileChecksums{}, pkgClient)

	err := job.Upload(context.Background())
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open file") {
		t.Errorf("expected 'failed to open file' in error, got %v", err)
	}
}

func TestRawUpload_RespectsContextCancel(t *testing.T) {
	stop := make(chan struct{})
	_, _ = withFakePkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	})
	t.Cleanup(func() { close(stop) })

	path, size := writeTempFile(t, "data")
	pkgClient := createTestRawPkgClient(t)
	job := NewRawUploadJob("blob.bin", path, "blob.bin", "myreg", size, utils.FileChecksums{}, pkgClient)

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
		if !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("expected context cancellation error, got %v", err)
		}
	}
}
