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

type RawUploadJob struct {
	BaseFileUploadJob
	RegistryName string
	// DestPath is the raw file path as it will appear in the URL after /files/.
	// Forward slashes only, no leading slash.
	DestPath  string
	Checksums utils.FileChecksums
	PkgClient *pkgclient.ClientWithResponses
}

// NewRawUploadJob creates a new raw upload job.
func NewRawUploadJob(id, filePath, destPath, registry string, fileSize int64, checksums utils.FileChecksums, pkgClient *pkgclient.ClientWithResponses) *RawUploadJob {
	return &RawUploadJob{
		BaseFileUploadJob: BaseFileUploadJob{
			ID:       id,
			FilePath: filePath,
			FileSize: fileSize,
		},
		RegistryName: registry,
		DestPath:     destPath,
		Checksums:    checksums,
		PkgClient:    pkgClient,
	}
}

// Upload performs the PUT upload of the raw file to its destination path.
func (j *RawUploadJob) Upload(ctx context.Context) error {
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
