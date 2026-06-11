package upload

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
)

// Generic Package Upload Job
type GenericUploadJob struct {
	BaseFileUploadJob
	RegistryName string
	PackageName  string
	Version      string
	// DestPath is the path the file lands at on the registry, exactly as it
	// will appear in the URL. Forward slashes only.
	DestPath  string
	Checksums utils.FileChecksums
	PkgClient *pkgclient.ClientWithResponses
}

// NewGenericUploadJob creates a new generic upload job
func NewGenericUploadJob(id, filePath, destPath, registry, packageName, version string, fileSize int64, checksums utils.FileChecksums, pkgClient *pkgclient.ClientWithResponses) *GenericUploadJob {
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
		PkgClient:    pkgClient,
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

	resp, err := j.PkgClient.UploadGenericFileToPathWithBodyWithResponse(
		ctx,
		config.Global.AccountID,
		j.RegistryName,
		j.DestPath,
		"application/octet-stream",
		file,
		func(ctx context.Context, req *http.Request) error {
			utils.SetChecksumHeaders(req.Header, j.Checksums)
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("upload failed with status %s: %s", resp.Status(), string(resp.Body))
	}
	return nil
}
