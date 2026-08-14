package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harness/harness-cli/config"
)

// A single shared *http.Client is used for every download job so that TCP+TLS
// connections are pooled across sequential downloads on a worker instead of
// each job opening (and holding) its own — which at scale would waste TLS
// handshakes and risk exhausting the process file-descriptor limit.
var (
	sharedClientOnce sync.Once
	sharedClient     *http.Client
)

func getSharedClient() *http.Client {
	sharedClientOnce.Do(func() {
		sharedClient = authAwareHTTPClient()
	})
	return sharedClient
}

// resetSharedClientForTest resets the shared client singleton so a test can
// exercise the client with a different config. Not called from production.
func resetSharedClientForTest() {
	sharedClientOnce = sync.Once{}
	sharedClient = nil
}

// authAwareHTTPClient returns an *http.Client with two safeguards:
//   - CheckRedirect strips custom auth headers (x-api-key, Authorization)
//     on cross-host redirects. Go's default policy drops Authorization/Cookie
//     but copies custom headers verbatim, so an API-host downloadUrl that
//     302s to presigned S3/GCS would otherwise ship the credential to the
//     storage provider.
//   - Transport.ResponseHeaderTimeout bounds the time we wait for response
//     headers from an unresponsive host so one stalled URL can't wedge a
//     worker indefinitely. Body streaming stays uncapped so slow-but-
//     progressing large artifact transfers aren't cut off.
// defaultResponseHeaderTimeoutSeconds is the fallback header-wait bound used
// when config.Global.TimeoutSeconds is 0 (the default). Without a floor,
// ResponseHeaderTimeout would be 0 (no timeout), letting an unresponsive
// host wedge a worker forever.
const defaultResponseHeaderTimeoutSeconds = 30

func authAwareHTTPClient() *http.Client {
	timeout := time.Duration(config.Global.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultResponseHeaderTimeoutSeconds * time.Second
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			// Go's default MaxIdleConnsPerHost is 2, which forces extra workers
			// to re-do TLS handshakes on every download and defeats the shared
			// client's connection pooling. Raise it so a full worker pool can
			// keep its connections alive.
			MaxIdleConnsPerHost:   100,
			ResponseHeaderTimeout: timeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			// Drop custom auth headers on any redirect that changes host OR
			// downgrades scheme (e.g. https→http same-host), since either
			// path lets credentials leave the trusted context.
			if !strings.EqualFold(req.URL.Host, via[0].URL.Host) ||
				!strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme) {
				req.Header.Del("x-api-key")
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
}

// ResolveDestPath returns the OS-native filesystem path where a file with the
// given registry path would be written under destDir.
func ResolveDestPath(destDir, registryPath string) string {
	return filepath.Join(destDir, filepath.FromSlash(registryPath))
}

// IsWithinDest reports whether a registry path resolves to a location inside
// destDir. Used to reject crafted paths like "../../evil.txt" before they can
// escape the download destination.
func IsWithinDest(destDir, registryPath string) bool {
	destPath := ResolveDestPath(destDir, registryPath)
	rel, err := filepath.Rel(destDir, destPath)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// FileDownloadJob represents a single file download operation.
type FileDownloadJob interface {
	GetID() string
	GetFilePath() string
	GetFileSize() int64
	Download(ctx context.Context) error
}

// BaseFileDownloadJob provides common fields for download jobs.
type BaseFileDownloadJob struct {
	ID       string
	FilePath string
	FileSize int64
}

func (b *BaseFileDownloadJob) GetID() string       { return b.ID }
func (b *BaseFileDownloadJob) GetFilePath() string { return b.FilePath }
func (b *BaseFileDownloadJob) GetFileSize() int64  { return b.FileSize }

// FileDownloadResult holds the outcome of one download job.
type FileDownloadResult struct {
	JobID    string
	FilePath string
	FileSize int64
	Error    error
	Success  bool
}

// URLDownloadJob downloads a single file from a direct URL.
// It works for any artifact type since it uses the downloadUrl
// returned by the search API rather than a type-specific client.
type URLDownloadJob struct {
	BaseFileDownloadJob
	RegistryPath string
	DestDir      string
	DownloadURL  string
	HTTPClient   *http.Client
}

func NewURLDownloadJob(id, registryPath, destDir, downloadURL string, fileSize int64) *URLDownloadJob {
	return &URLDownloadJob{
		BaseFileDownloadJob: BaseFileDownloadJob{
			ID:       id,
			FilePath: filepath.Join(destDir, filepath.FromSlash(registryPath)),
			FileSize: fileSize,
		},
		RegistryPath: registryPath,
		DestDir:      destDir,
		DownloadURL:  downloadURL,
		HTTPClient:   getSharedClient(),
	}
}

// isSameHostAsAPI reports whether downloadURL matches the configured API
// host AND scheme. Requiring the scheme match prevents an http:// downloadUrl
// (or a compromised registry crafting one) from causing us to transmit the
// API key over plaintext.
func isSameHostAsAPI(downloadURL string) bool {
	dl, err := url.Parse(downloadURL)
	if err != nil || dl.Host == "" {
		return false
	}
	api, err := url.Parse(config.Global.APIBaseURL)
	if err != nil || api.Host == "" {
		return false
	}
	return strings.EqualFold(dl.Host, api.Host) && strings.EqualFold(dl.Scheme, api.Scheme)
}

func (j *URLDownloadJob) Download(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// Only attach the credential when the download URL host matches the configured
	// API host. This prevents the API key from leaking to third-party hosts, such
	// as presigned blob storage URLs the server may redirect to, or an attacker
	// host if a compromised registry returns a crafted downloadUrl.
	if isSameHostAsAPI(j.DownloadURL) {
		if strings.HasPrefix(config.Global.AuthToken, "CIManager") {
			req.Header.Set("Authorization", config.Global.AuthToken)
		} else {
			req.Header.Set("x-api-key", config.Global.AuthToken)
		}
	}

	resp, err := j.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %s: %s", resp.Status, string(body))
	}

	destPath := ResolveDestPath(j.DestDir, j.RegistryPath)
	if !IsWithinDest(j.DestDir, j.RegistryPath) {
		return fmt.Errorf("refusing to write outside destination dir: %s", j.RegistryPath)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", destPath, err)
	}

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		_ = outFile.Close()
		_ = os.Remove(destPath)
		return fmt.Errorf("failed to write file %s: %w", destPath, err)
	}

	if err := outFile.Close(); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("failed to close file %s: %w", destPath, err)
	}

	return nil
}
