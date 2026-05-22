package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
)

// Python package upload job
type PythonUploadJob struct {
	BaseFileUploadJob
	FilePath     string
	RegistryName string
	PackageName  string
	Version      string
	PkgClient    *pkgclient.ClientWithResponses
}

// NewPythonUploadJob creates a new Python upload job
func NewPythonUploadJob(filePath, registryName, packageName, version string, fileSize int64, client *pkgclient.ClientWithResponses) *PythonUploadJob {
	return &PythonUploadJob{
		BaseFileUploadJob: BaseFileUploadJob{
			ID:       filepath.Base(filePath),
			FilePath: filePath,
			FileSize: fileSize,
		},
		FilePath:     filePath,
		RegistryName: registryName,
		PackageName:  packageName,
		Version:      version,
		PkgClient:    client,
	}
}

// Python package upload
func (j *PythonUploadJob) Upload(ctx context.Context) error {
	pkgClient := j.PkgClient
	if pkgClient == nil {
		return fmt.Errorf("package client not initialized")
	}

	file, err := os.Open(j.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open package file: %w", err)
	}
	defer file.Close()

	var formData bytes.Buffer
	fileWriter := multipart.NewWriter(&formData)

	err = fileWriter.WriteField("name", j.PackageName)
	if err != nil {
		return err
	}
	err = fileWriter.WriteField("version", j.Version)
	if err != nil {
		return err
	}

	part, err := fileWriter.CreateFormFile("content", filepath.Base(j.FilePath))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	fileWriter.Close()

	resp, err := pkgClient.UploadPythonPackageWithBodyWithResponse(
		ctx,
		config.Global.AccountID,
		j.RegistryName,
		fileWriter.FormDataContentType(),
		&formData,
	)

	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("upload failed with status %s: %s", resp.Status(), string(resp.Body))
	}

	return nil
}
