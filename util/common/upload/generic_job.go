package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/common/auth"
)

// Generic Package Upload Job
type GenericUploadJob struct {
	BaseFileUploadJob
	RegistryName string
	PackageName  string
	Version      string
	// DestPath is the path the file lands at on the registry, exactly as it
	// will appear in the URL. Forward slashes only.
	DestPath   string
	Checksums  utils.FileChecksums
	PkgBaseURL string
	HTTPClient *http.Client
}

// NewGenericUploadJob creates a new generic upload job
func NewGenericUploadJob(id, filePath, destPath, registry, packageName, version string, fileSize int64, checksums utils.FileChecksums, pkgBaseURL string, httpClient *http.Client) *GenericUploadJob {
	return &GenericUploadJob{
		BaseFileUploadJob: BaseFileUploadJob{
			ID:       id,
			FilePath: filePath,
			FileSize: fileSize,
		},
		RegistryName: registry,
		PackageName:  packageName,
		Version:      version,
		DestPath:     destPath,
		Checksums:    checksums,
		PkgBaseURL:   pkgBaseURL,
		HTTPClient:   httpClient,
	}
}

// Upload performs the PUT once. Any transient-failure handling is the
// HTTP client layer's responsibility (see comment on GenericUploadJob).
func (j *GenericUploadJob) Upload(ctx context.Context) error {
	file, err := os.Open(j.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", j.FilePath, err)
	}
	defer file.Close()

	// Build the URL directly to avoid the generated client encoding slashes in
	// the multi-segment filepath as %2F.
	url := fmt.Sprintf("%s/pkg/%s/%s/files/%s",
		strings.TrimRight(j.PkgBaseURL, "/"),
		config.Global.AccountID,
		j.RegistryName,
		strings.TrimPrefix(j.DestPath, "/"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-api-key", config.Global.AuthToken)
	if strings.HasPrefix(config.Global.AuthToken, auth.JWTTokenPrefix) {
		req.Header.Set("Authorization", config.Global.AuthToken)
	}
	utils.SetChecksumHeaders(req.Header, j.Checksums)

	resp, err := j.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %s: %s", resp.Status, string(body))
	}
	return nil
}
