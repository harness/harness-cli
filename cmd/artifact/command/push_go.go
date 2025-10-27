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

	"github.com/harness/harness-cli/config"
	client "github.com/harness/harness-cli/internal/api/ar"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/module/ar/packages/gopkg"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/errors"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewPushGoCmd(c *client.ClientWithResponses) *cobra.Command {
	var dir = "."
	var output = "/tmp/go-package"
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "go <registry_name> <version>",
		Short: "Push Go Artifacts",
		Long:  "Push Go Artifacts to Harness Artifact Registry",
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
			version := args[1]

			// Create progress reporter
			progress := p.NewConsoleReporter()

			// Validate Registry Name and Version
			progress.Start("Validating input parameters")
			if registryName == "" {
				progress.Error("Registry name is required")
				return errors.NewValidationError("registry_name", "registry name is required")
			}
			if version == "" {
				progress.Error("Version is required")
				return errors.NewValidationError("version", "version is required")
			}
			progress.Success("Input parameters validated")

			// Generate package files
			generator := gopkg.NewGenerator(dir, output, version)
			packageName, err := generator.Generate(progress)
			if err != nil {
				return err
			}

			// Create form data
			progress.Step("Preparing package upload")
			var formData bytes.Buffer
			var formWriter = multipart.NewWriter(&formData)

			// Add files to form
			var files = []struct {
				name     string
				filename string
			}{
				{"mod", packageName + ".mod"},
				{"info", packageName + ".info"},
				{"zip", packageName + ".zip"},
			}

			// Upload files
			progress.Step("Preparing package files")
			for _, file := range files {
				progress.Step(fmt.Sprintf("Adding %s to upload", file.filename))
				f, openErr := os.Open(filepath.Join(output, file.filename))
				if openErr != nil {
					progress.Error(fmt.Sprintf("Failed to open %s", file.filename))
					return openErr
				}
				defer f.Close()

				part, formErr := formWriter.CreateFormFile(file.name, file.filename)
				if formErr != nil {
					progress.Error(fmt.Sprintf("Failed to create form field for %s", file.filename))
					return formErr
				}

				if _, copyErr := io.Copy(part, f); copyErr != nil {
					progress.Error(fmt.Sprintf("Failed to copy %s content", file.filename))
					return copyErr
				}
			}

			// Close multipart writer
			if closeErr := formWriter.Close(); closeErr != nil {
				progress.Error("Failed to finalize form data")
				return closeErr
			}

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetXApiKeyOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			// Upload package
			progress.Step("Uploading package to registry")
			bufferSize := int64(formData.Len())
			reader, closer := p.Reader(bufferSize, &formData, "go")
			defer closer()

			resp, err := pkgClient.UploadGoPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				formWriter.FormDataContentType(),
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

			progress.Success(fmt.Sprintf("Successfully uploaded package %s", packageName))
			return nil
		},
	}

	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	return cmd
}
