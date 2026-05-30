package upload

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
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
	DestPath string
}

// NewGenericUploadJob creates a new generic upload job
func NewGenericUploadJob(id, filePath, destPath, registry, packageName, version string, fileSize int64) *GenericUploadJob {
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
	}
}

// Upload performs the PUT once. Any transient-failure handling is the
// HTTP client layer's responsibility (see comment on GenericUploadJob).
func (j *GenericUploadJob) Upload(ctx context.Context) error {
	pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		return fmt.Errorf("failed to create package client: %w", err)
	}

	file, err := os.Open(j.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", j.FilePath, err)
	}
	defer file.Close()

	resp, err := pkgClient.UploadGenericFileToPathWithBodyWithResponse(
		ctx,
		config.Global.AccountID,
		j.RegistryName,
		j.DestPath,
		"application/octet-stream",
		file,
	)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("upload failed with status %s: %s", resp.Status(), string(resp.Body))
	}
	return nil
}
