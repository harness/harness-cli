package upload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

// withPythonPkgServer spins up a stub package server, points config.Global at
// it, and restores all globals on cleanup. The handler receives the
// per-request hit count (1-indexed) so tests can encode different responses
// per attempt declaratively.
func withPythonPkgServer(t *testing.T, handler func(hit int, w http.ResponseWriter, r *http.Request)) (server *httptest.Server, hits *int64) {
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

func writePythonFile(t *testing.T, contents string) (string, int64) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.whl")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p, int64(len(contents))
}

func createTestPythonPkgClient(t *testing.T) *pkgclient.ClientWithResponses {
	t.Helper()
	client, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return client
}

func TestPythonUpload_Success(t *testing.T) {
	_, hits := withPythonPkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Verify checksum headers are present
		if r.Header.Get("X-Checksum-Md5") == "" {
			t.Error("X-Checksum-Md5 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha1") == "" {
			t.Error("X-Checksum-Sha1 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha256") == "" {
			t.Error("X-Checksum-Sha256 header is missing")
		}
		if r.Header.Get("X-Checksum-Sha512") == "" {
			t.Error("X-Checksum-Sha512 header is missing")
		}
		w.WriteHeader(http.StatusOK)
	})

	path, size := writePythonFile(t, "hello world")
	checksums := utils.FileChecksums{
		MD5:    "5eb63bbbbe01eeed093cb22bb8f5acdc",
		SHA1:   "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
		SHA256: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		SHA512: "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f",
	}
	pkgClient := createTestPythonPkgClient(t)
	job := NewPythonUploadJob(path, "myreg", "test-pkg", "1.0.0", size, checksums, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("expected exactly 1 server hit, got %d", got)
	}
}

func TestPythonUpload_Success_201Created(t *testing.T) {
	_, _ = withPythonPkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	path, size := writePythonFile(t, "data")
	checksums := utils.FileChecksums{
		MD5:    "8d777f385d3dfec8815d20f7496026dc",
		SHA1:   "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		SHA256: "3a7bd3e2360a3d29eea436fcfb7e44c735d117c4d1eef568a0e0b0c8b1301a70",
		SHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	}
	pkgClient := createTestPythonPkgClient(t)
	job := NewPythonUploadJob(path, "myreg", "test-pkg", "1.0.0", size, checksums, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("expected success on 201, got %v", err)
	}
}

func TestPythonUpload_FailsOn4xx(t *testing.T) {
	_, hits := withPythonPkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	path, size := writePythonFile(t, "data")
	checksums := utils.FileChecksums{
		MD5:    "8d777f385d3dfec8815d20f7496026dc",
		SHA1:   "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		SHA256: "3a7bd3e2360a3d29eea436fcfb7e44c735d117c4d1eef568a0e0b0c8b1301a70",
		SHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	}
	pkgClient := createTestPythonPkgClient(t)
	job := NewPythonUploadJob(path, "myreg", "test-pkg", "1.0.0", size, checksums, pkgClient)

	err := job.Upload(context.Background())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("expected exactly 1 server hit, got %d", got)
	}
}

func TestPythonUpload_FailsOn5xx(t *testing.T) {
	_, _ = withPythonPkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	path, size := writePythonFile(t, "data")
	checksums := utils.FileChecksums{
		MD5:    "8d777f385d3dfec8815d20f7496026dc",
		SHA1:   "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		SHA256: "3a7bd3e2360a3d29eea436fcfb7e44c735d117c4d1eef568a0e0b0c8b1301a70",
		SHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	}
	pkgClient := createTestPythonPkgClient(t)
	job := NewPythonUploadJob(path, "myreg", "test-pkg", "1.0.0", size, checksums, pkgClient)

	if err := job.Upload(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestPythonUpload_FailsOnMissingFile(t *testing.T) {
	_, _ = withPythonPkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be hit when file is missing")
	})

	checksums := utils.FileChecksums{}
	pkgClient := createTestPythonPkgClient(t)
	job := NewPythonUploadJob("/path/that/does/not/exist.whl", "myreg", "test-pkg", "1.0.0", 0, checksums, pkgClient)

	err := job.Upload(context.Background())
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open package file") {
		t.Errorf("expected 'failed to open package file' in error, got %v", err)
	}
}

func TestPythonUpload_ChecksumHeadersSet(t *testing.T) {
	receivedHeaders := make(http.Header)
	_, _ = withPythonPkgServer(t, func(hit int, w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			for _, value := range values {
				receivedHeaders.Add(key, value)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	path, size := writePythonFile(t, "test content for checksums")
	checksums := utils.FileChecksums{
		MD5:    "d41d8cd98f00b204e9800998ecf8427e",
		SHA1:   "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	}
	pkgClient := createTestPythonPkgClient(t)
	job := NewPythonUploadJob(path, "myreg", "test-pkg", "1.0.0", size, checksums, pkgClient)

	if err := job.Upload(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify checksum headers are set
	if receivedHeaders.Get("X-Checksum-Md5") == "" {
		t.Error("X-Checksum-Md5 header was not set")
	}
	if receivedHeaders.Get("X-Checksum-Sha1") == "" {
		t.Error("X-Checksum-Sha1 header was not set")
	}
	if receivedHeaders.Get("X-Checksum-Sha256") == "" {
		t.Error("X-Checksum-Sha256 header was not set")
	}
	if receivedHeaders.Get("X-Checksum-Sha512") == "" {
		t.Error("X-Checksum-Sha512 header was not set")
	}
}
