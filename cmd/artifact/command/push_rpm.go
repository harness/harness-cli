package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

const (
	RpmFileExtension = ".rpm"
)

func NewPushRpmCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "rpm <registry_name> <file_path>",
		Short: "Push Rpm Artifacts",
		Long:  "Push Rpm Artifacts to Harness Artifact Registry",
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
			valid, err := fileutil.IsFilenameAcceptable(fileName, RpmFileExtension)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
			}

			progress.Success("Input parameters validated")

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetAuthOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			file, err := os.Open(filePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return err
			}
			defer file.Close()

			var formData bytes.Buffer
			fileWriter := multipart.NewWriter(&formData)

			// Create the form field "file" to match curl
			part, err := fileWriter.CreateFormFile("file", filepath.Base(filePath))
			if err != nil {
				return err
			}

			// Copy the file into the multipart field
			_, err = io.Copy(part, file)
			if err != nil {
				return err
			}

			fileWriter.Close()

			// Initialize progress reader
			progress.Step("Uploading package to registry")
			bufferSize := int64(formData.Len())
			reader, closer := p.Reader(bufferSize, &formData, "rpm")
			defer closer()

			resp, err := pkgClient.UploadRpmPackageWithBodyWithResponse(
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
