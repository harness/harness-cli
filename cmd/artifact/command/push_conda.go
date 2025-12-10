package command

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/errors"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"
	"github.com/zhyee/zipstream"
)

const (
	CondaFileExtension = ".conda"
	Bz2FileExtension   = ".tar.bz2"
)

func NewPushCondaCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	var customHeaders map[string]string
	cmd := &cobra.Command{
		Use:   "conda <registry_name> <file_path>",
		Short: "Push Conda Artifacts",
		Long:  "Push Conda Artifacts to Harness Artifact Registry",
		Args:  cobra.ExactArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			if pkgURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURL)
			} else {
				config.Global.Registry.PkgURL = util.GetPkgUrl(config.Global.APIBaseURL)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			filePath := args[1]
			fileName := filepath.Base(filePath)

			// Create progress reporter
			progress := p.NewConsoleReporter()

			// Validate Registry Name and file_path
			progress.Start("Validating input parameters")
			if registryName == "" {
				progress.Error("Registry name is required")
				return errors.NewValidationError("registry_name", "registry name is required")
			}
			if filePath == "" {
				progress.Error("File path is required")
				return errors.NewValidationError("file_path", "file path is required")
			}

			// Validate file exists
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
			}
			if fileInfo.IsDir() {
				return errors.NewValidationError("file_path", "package file path must be a file, not a directory")
			}

			// validate file name
			valid, err := validateFileName(fileName)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
			}

			// get metadata from file
			metadata, err := GetMetadataFromPayload(filePath, fileName)
			if err != nil {
				progress.Error("Failed to get metadata from payload")
				return err
			}

			progress.Success("Input parameters validated")

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetXApiKeyOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			// Upload package
			progress.Step("Uploading package to registry")
			// upload file from filepath to registry using pkgClient.UploadCondaPackageWithBodyWithResponse

			file, err := os.Open(filePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return err
			}
			defer file.Close()

			// Initialize progress reader
			bufferSize := int64(fileInfo.Size())
			reader, closer := p.Reader(bufferSize, file, "conda")
			defer closer()

			// Initialize customHeaders if nil
			if customHeaders == nil {
				customHeaders = make(map[string]string)
			}

			// add X-File-Name in header
			customHeaders["X-File-Name"] = filepath.Base(filePath)

			// add X-Subdir in header
			customHeaders["X-Subdir"] = metadata.Subdir

			// Create custom header editor function
			customHeaderEditor := func(ctx context.Context, req *http.Request) error {
				// Add custom headers from the map
				for key, value := range customHeaders {
					req.Header.Set(key, value)
				}
				return nil
			}

			resp, err := pkgClient.UploadCondaPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				"application/octet-stream",
				reader,
				customHeaderEditor,
			)

			if err != nil {
				progress.Error("Failed to upload package")
				return err
			}
			// Check response
			if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
				progress.Error("Upload failed")
				return fmt.Errorf("failed to push package: %s \n response: %s", resp.Status(), resp.Body)
			}

			progress.Success(fmt.Sprintf("Successfully uploaded package %s", filePath))
			return nil
		},
	}

	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	return cmd
}

func validateFileName(fileName string) (bool, error) {
	if fileName == "" {
		return false, fmt.Errorf("empty filename")
	}

	name := fileName
	switch {
	case strings.HasSuffix(name, CondaFileExtension):
		return true, nil
	case strings.HasSuffix(name, Bz2FileExtension):
		return true, nil
	default:
		return false, fmt.Errorf("unsupported extension: %s", filepath.Ext(name))
	}
}

type VersionInfo struct {
	Description       string   `json:"description,omitempty"`
	Summary           string   `json:"summary,omitempty"`
	Homepage          string   `json:"home,omitempty"`
	Repository        string   `json:"dev_url,omitempty"`             //nolint:tagliatelle
	Documentation     string   `json:"doc_url,omitempty"`             //nolint:tagliatelle
	CondaVersion      string   `json:"conda_version,omitempty"`       //nolint:tagliatelle
	CondaBuildVersion string   `json:"conda_build_version,omitempty"` //nolint:tagliatelle
	Tags              []string `json:"tags,omitempty"`
	Readme            string   `json:"readme,omitempty"`
}

type VersionMetadata struct {
	VersionInfo
	Architecture  string   `json:"arch"`
	Build         string   `json:"build"`
	BuildNumber   int64    `json:"build_number"` //nolint:tagliatelle
	Dependencies  []string `json:"depends"`
	License       string   `json:"license"`
	LicenseFamily string   `json:"license_family"` //nolint:tagliatelle
	Name          string   `json:"name"`
	Platform      string   `json:"platform"`
	Subdir        string   `json:"subdir"`
	Timestamp     int64    `json:"timestamp"`
	Version       string   `json:"version"`
	MD5           string   `json:"md5"`
	Sha256        string   `json:"sha256"`
	Size          int64    `json:"size"`
}

func GetMetadataFromPayload(
	filePath string, fileName string,
) (*VersionMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	switch {
	case strings.HasSuffix(fileName, CondaFileExtension):
		return ParseMetadataFromCondaPayload(file)
	case strings.HasSuffix(fileName, Bz2FileExtension):
		return ParseMetadataFromBZ2Payload(file)
	}
	return nil, fmt.Errorf("unknown file extension: %s", filepath.Ext(fileName))
}

func ParseMetadataFromCondaPayload(reader io.Reader) (
	*VersionMetadata,
	error,
) {
	// Use zipstream for streaming zip reading without loading entire file into memory
	zipReader := zipstream.NewReader(reader)
	for {
		entry, err := zipReader.GetNextEntry()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read zip entry: %w", err)
		}

		// Match info-*.tar.zst pattern (standard .conda format)
		if strings.HasPrefix(entry.Name, "info-") && strings.HasSuffix(entry.Name, ".tar.zst") {
			// Read the compressed entry data from the zip stream
			entryReader, err := entry.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open zip entry: %w", err)
			}
			defer entryReader.Close()

			// Create zstd reader for decompression
			zstdReader, err := zstd.NewReader(entryReader)
			if err != nil {
				return nil, fmt.Errorf("failed to create zstd reader: %w", err)
			}
			defer zstdReader.Close()

			return ParseMetadataFromPayload(zstdReader)
		}
	}
	return nil, fmt.Errorf("failed to find metadata file (info-*.tar.zst) in .conda archive")
}

func ParseMetadataFromBZ2Payload(reader io.Reader) (
	*VersionMetadata,
	error,
) {
	return ParseMetadataFromPayload(bzip2.NewReader(reader))
}

func ParseMetadataFromPayload(reader io.Reader) (
	*VersionMetadata,
	error,
) {
	metadata := &VersionMetadata{}
	isFoundIndexFile, isFoundAboutFile := false, false

	// Create a tar reader
	tarReader := tar.NewReader(io.LimitReader(reader, 1073741824)) // 1GB limit

	// Iterate over files in tar
	for {
		hdr, err := tarReader.Next()
		if isFoundIndexFile && isFoundAboutFile {
			break
		}
		if err == io.EOF {
			break // end of archive
		}
		if err != nil {
			return nil, err
		}
		if !isFoundIndexFile && strings.Contains(strings.ToLower(hdr.Name), "index.json") {
			if err := json.NewDecoder(tarReader).Decode(metadata); err != nil {
				return nil, err
			}
			isFoundIndexFile = true
		}
		if !isFoundAboutFile && strings.Contains(strings.ToLower(hdr.Name), "about.json") {
			about := &VersionInfo{}
			if err := json.NewDecoder(tarReader).Decode(about); err != nil {
				return nil, err
			}
			metadata.VersionInfo = *about
			isFoundAboutFile = true
		}
	}

	return metadata, nil
}
