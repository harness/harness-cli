package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
)

type MavenUploadJob struct {
	BaseFileUploadJob
	RegistryName string
	GroupID      string
	ArtifactID   string
	Version      string
	FileName     string
	FileContent  *FileContent
}

// creates a Maven upload job from a file on disk
func NewMavenUploadJobFromDisk(filePath, fileName, registryName, groupID, artifactID, version string, fileSize int64) *MavenUploadJob {
	return &MavenUploadJob{
		BaseFileUploadJob: BaseFileUploadJob{
			ID:       fileName,
			FilePath: filePath,
			FileSize: fileSize,
		},
		RegistryName: registryName,
		GroupID:      groupID,
		ArtifactID:   artifactID,
		Version:      version,
		FileName:     fileName,
		FileContent:  nil,
	}
}

// using for checkusm , as that is inMemory
func NewMavenUploadJobFromMemory(fileName, registryName, groupID, artifactID, version string, data []byte) *MavenUploadJob {
	return &MavenUploadJob{
		BaseFileUploadJob: BaseFileUploadJob{
			ID:       fileName,
			FilePath: fileName,
			FileSize: int64(len(data)),
		},
		RegistryName: registryName,
		GroupID:      groupID,
		ArtifactID:   artifactID,
		Version:      version,
		FileName:     fileName,
		FileContent:  NewFileContentFromMemory(data),
	}
}

// performs the Maven artifact upload
func (j *MavenUploadJob) Upload(ctx context.Context) error {
	pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		return fmt.Errorf("failed to create package client: %w", err)
	}

	var reader io.Reader

	// checksum file handling here
	if j.FileContent != nil && j.FileContent.IsInMemory {
		reader = bytes.NewReader(j.FileContent.Data)
	} else {
		// Load from disk
		file, err := os.Open(j.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		reader = file
	}

	resp, err := pkgClient.UploadMavenPackageWithBodyWithResponse(
		ctx,
		config.Global.AccountID,
		j.RegistryName,
		j.GroupID,
		j.ArtifactID,
		j.Version,
		j.FileName,
		"application/octet-stream",
		reader,
	)

	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}

	if resp.StatusCode()/100 != 2 {
		return fmt.Errorf("upload failed with status %s: %s", resp.Status(), string(resp.Body))
	}

	return nil
}

// NormalizePomFilename converts pom.xml to Maven build .pom structure
func NormalizePomFilename(artifactID, version string) string {
	return fmt.Sprintf("%s-%s.pom", artifactID, version)
}

// NormalizeFileName handles special cases like pom.xml
func NormalizeFileName(filePath string, artifactID, version string) string {
	fileName := filepath.Base(filePath)
	if fileName == "pom.xml" {
		return NormalizePomFilename(artifactID, version)
	}
	return fileName
}
