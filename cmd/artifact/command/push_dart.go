package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/module/ar/migrate/types/dart"
	"github.com/harness/harness-cli/util/common/auth"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewPushDartCmd creates a new cobra.Command for pushing Dart packages.
// Command example: hc artifact push dart <registry_name> <dart_tar_gz_path>
func NewPushDartCmd(f *cmdutils.Factory) *cobra.Command {
	var pkgURL string

	cmd := &cobra.Command{
		Use:   "dart <registry_name> <dart_tar_gz_path>",
		Short: "Push Dart package",
		Long:  "Push a Dart .tar.gz package to Harness Artifact Registry (HAR)",
		Args:  cobra.ExactArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			config.Global.Registry.PkgURL = pkgURL
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			packageFilePath := args[1]

			// Create progress reporter
			progress := p.NewConsoleReporter()

			// Validate input parameters
			progress.Start("Validating input parameters")
			if registryName == "" {
				progress.Error("Registry name is required")
				return fmt.Errorf("registry name is required")
			}
			if packageFilePath == "" {
				progress.Error("Package file path is required")
				return fmt.Errorf("package file path is required")
			}

			fileInfo, err := os.Stat(packageFilePath)
			if err != nil {
				progress.Error("Failed to access package file")
				return fmt.Errorf("failed to access package file: %w", err)
			}
			if fileInfo.IsDir() {
				progress.Error("Package file path must be a file, not a directory")
				return errors.New("package file path must be a file, not a directory")
			}

			ext := filepath.Ext(packageFilePath)
			if ext != ".gz" && ext != ".tgz" {
				progress.Error(fmt.Sprintf("Package file must be a .tar.gz or .tgz file, got: %s", ext))
				return fmt.Errorf("package file must be a .tar.gz or .tgz file, got: %s", ext)
			}
			progress.Success("Input parameters validated")

			// Extract pubspec.yaml from tarball
			progress.Step("Extracting pubspec.yaml from tarball")
			pubspecBytes, err := extractPubspecFromTarball(packageFilePath)
			if err != nil {
				progress.Error("Failed to extract pubspec.yaml from tarball")
				return fmt.Errorf("failed to extract pubspec.yaml from tarball: %w", err)
			}

			// Parse pubspec.yaml
			progress.Step("Parsing pubspec.yaml")
			pubspec, err := parsePubspec(pubspecBytes)
			if err != nil {
				progress.Error("Failed to parse pubspec.yaml")
				return fmt.Errorf("failed to parse pubspec.yaml: %w", err)
			}

			if pubspec.Name == "" || pubspec.Version == "" {
				progress.Error("Pubspec.yaml must contain non-empty 'name' and 'version'")
				return fmt.Errorf("pubspec.yaml must contain non-empty 'name' and 'version'")
			}

			if config.Global.Registry.PkgURL == "" {
				progress.Error("pkg-url must be set")
				return fmt.Errorf("pkg-url must be set")
			}
			progress.Success(fmt.Sprintf("Package metadata extracted: %s@%s", pubspec.Name, pubspec.Version))

			// Initialize the package client
			progress.Step("Initializing package client")
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				progress.Error("Failed to create package client")
				return fmt.Errorf("failed to create package client: %w", err)
			}

			// Open the tar.gz file for upload
			progress.Step("Preparing package file for upload")
			file, err := os.Open(packageFilePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return fmt.Errorf("failed to open package file: %w", err)
			}
			defer file.Close()

			uploadID := uuid.New().String()

			// Create a pipe for streaming multipart form data
			progress.Step("Preparing multipart upload")
			pr, pw := io.Pipe()
			writer := multipart.NewWriter(pw)

			// Write multipart form in a goroutine
			go func() {
				defer pw.Close()
				defer writer.Close()

				// Create form file field "file"
				part, err := writer.CreateFormFile("file", filepath.Base(packageFilePath))
				if err != nil {
					pw.CloseWithError(fmt.Errorf("failed to create form file: %w", err))
					return
				}

				// Copy file content to the form field
				if _, err := io.Copy(part, file); err != nil {
					pw.CloseWithError(fmt.Errorf("failed to write file to form: %w", err))
					return
				}
			}()

			// Upload package using generated client with progress tracking
			progress.Step("Uploading package to registry")

			// Initialize progress reader for upload tracking
			bufferSize := fileInfo.Size()
			reader, closer := p.Reader(bufferSize, pr, fileInfo.Name())
			defer closer()

			resp, err := pkgClient.UploadDartPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				uploadID,
				writer.FormDataContentType(),
				reader,
			)
			if err != nil {
				progress.Error("Failed to upload Dart package")
				return fmt.Errorf("failed to upload Dart package: %w", err)
			}

			// Check response
			if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusNoContent {
				progress.Error("Upload failed")
				return fmt.Errorf("failed to upload Dart package: %s \n response: %s", resp.Status(), resp.Body)
			}

			progress.Success(fmt.Sprintf("Successfully uploaded Dart package '%s@%s' to registry '%s'", pubspec.Name, pubspec.Version, registryName))
			return nil
		},
	}

	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages service")
	cmd.MarkFlagRequired("pkg-url")

	return cmd
}

// extractPubspecFromTarball extracts pubspec.yaml from a Dart .tar.gz package
func extractPubspecFromTarball(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open tarball: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.FileInfo().IsDir() {
			continue
		}

		base := filepath.Base(header.Name)
		if base == "pubspec.yaml" {
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read pubspec.yaml from tarball: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("pubspec.yaml not found in tarball")
}

// parsePubspec parses pubspec.yaml bytes into a Pubspec struct
func parsePubspec(data []byte) (*dart.Pubspec, error) {
	var pubspec dart.Pubspec
	if err := yaml.Unmarshal(data, &pubspec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pubspec.yaml: %w", err)
	}
	return &pubspec, nil
}
