package command

import (
	"context"
	"fmt"
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

	"github.com/spf13/cobra"
)

const (
	ComposerFileExtension = ".zip"
)

func NewPushComposerCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "composer <registry_name> <file_path>",
		Short: "Push Composer Artifacts",
		Long:  "Push Composer Artifacts to Harness Artifact Registry",
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
			valid, err := validateComposerFileName(fileName)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
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

			file, err := os.Open(filePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return err
			}
			defer file.Close()

			// Initialize progress reader
			bufferSize := int64(fileInfo.Size())
			reader, closer := p.Reader(bufferSize, file, "composer")
			defer closer()

			resp, err := pkgClient.UploadComposerPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				"application/octet-stream",
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

func validateComposerFileName(fileName string) (bool, error) {
	if fileName == "" {
		return false, fmt.Errorf("empty filename")
	}

	if !strings.HasSuffix(fileName, ComposerFileExtension) {
		return false, fmt.Errorf("unsupported extension: %s, expected %s", filepath.Ext(fileName), ComposerFileExtension)
	}

	return true, nil
}
