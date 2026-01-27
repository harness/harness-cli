package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

const (
	CargoFileExtension = ".crate"
)

func NewPushCargoCmd(f *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "cargo <registry_name> <file_path>",
		Short: "Push Cargo Artifacts",
		Long:  "Push Cargo Artifacts to Harness Artifact Registry",
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

			fileName := filepath.Base(filePath)

			// Validate file exists
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
			}
			if fileInfo.IsDir() {
				return errors.NewValidationError("file_path", "package file path must be a file, not a directory")
			}

			// validate file name
			valid, err := fileutil.IsFilenameAcceptable(fileName, CargoFileExtension)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path",
					fmt.Sprintf("failed to validate package file name: %v", err))
			}

			metadata, err := getMetadataFromCrateFile(filePath)

			if err != nil {
				progress.Error("Failed to get metadata from payload")
				return err
			}

			packageName := metadata.Package.Name
			version := metadata.Package.Version

			if len(packageName) == 0 {
				return errors.NewValidationError("package_name", "Package name is not present in metadata")
			}
			if len(version) == 0 {
				return errors.NewValidationError("version", "Version is not present in metadata")
			}
			file, err := os.Open(filePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return err
			}

			fileData, err := os.ReadFile(filePath)

			if err != nil {
				return fmt.Errorf("failed to read file: %v", err)
			}

			payload, err := makeCargoPackagePayload(packageName, version, fileData)

			if err != nil {
				return fmt.Errorf("failed to create package payload: %v", err)
			}

			progress.Success("Input parameters validated")

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			defer file.Close()

			var formData bytes.Buffer
			fileWriter := multipart.NewWriter(&formData)

			part, err := fileWriter.CreateFormFile("file", filepath.Base(filePath))
			if err != nil {
				return err
			}

			_, err = io.Copy(part, bytes.NewReader(payload))
			if err != nil {
				return err
			}

			fileWriter.Close()

			// Initialize progress reader
			progress.Step("Uploading package to registry")
			bufferSize := int64(len(payload))

			reader, closer := p.Reader(bufferSize, bytes.NewReader(payload), "cargo")
			defer closer()

			resp, err := pkgClient.UploadCargoPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				fileWriter.FormDataContentType(),
				reader,
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

func getMetadataFromCrateFile(filePath string) (*CargoPackageMetadata, error) {
	file, err := os.Open(filePath)

	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return parseMetadataFromCrate(file)

}

func parseMetadataFromCrate(reader io.Reader) (*CargoPackageMetadata, error) {

	// Gzip decompression
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	// Tar reader
	tarReader := tar.NewReader(gz)
	metadata := &CargoPackageMetadata{}

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("package read error: %w", err)
		}

		name := path.Base(hdr.Name)

		if name == "Cargo.toml" {
			if _, err = toml.NewDecoder(tarReader).Decode(&metadata); err != nil {
				return nil, fmt.Errorf("failed parsing Cargo.toml: %w", err)
			}
			return metadata, nil // found and parsed successfully
		}
	}

	return nil, fmt.Errorf("meta data file Cargo.toml not found in crate")
}

func makeCargoPackagePayload(packageName string, version string, crateFile []byte) ([]byte, error) {
	// Create metadata for the package
	metadata := map[string]string{
		"name": packageName,
		"vers": version,
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("error marshaling metadata: %v", err)
	}

	metadataLen := uint32(len(metadataBytes)) // #nosec G115
	packageLen := uint32(len(crateFile))      // #nosec G115
	// Construct the request body according to the Cargo package upload format
	body := make([]byte, 4+metadataLen+4+packageLen)
	binary.LittleEndian.PutUint32(body[:4], metadataLen)
	copy(body[4:], metadataBytes)
	binary.LittleEndian.PutUint32(body[4+metadataLen:4+metadataLen+4], packageLen)
	copy(body[4+metadataLen+4:], crateFile)

	return body, nil
}

type CargoPackageMetadata struct {
	Package struct {
		Name    string `toml:"name"`
		Version string `toml:"version"`
	} `toml:"package"`
}
