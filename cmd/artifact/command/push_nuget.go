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

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
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
	NugetFileExtension    = ".nupkg"
	expectedArgumentCount = 2
)

func NewPushNugetCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	var path string
	cmd := &cobra.Command{
		Use:   "nuget <registry_name> <file_path>",
		Short: "Push Nuget Artifacts",
		Long:  "Push Nuget Artifacts to Harness Artifact Registry",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedArgumentCount {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					expectedArgumentCount, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
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

			// Resolve file path (supports glob patterns like *.nupkg)
			files, err := utils.ResolveFilePath(filePath, NugetFileExtension)
			if err != nil {
				progress.Error("Failed to resolve file path")
				return err
			}
			filePath = files[0]
			progress.Step(fmt.Sprintf("Uploading file: %s", filePath))

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
			valid, err := fileutil.IsFilenameAcceptable(fileName, NugetFileExtension)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
			}

			progress.Success("Input parameters validated")

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(
				config.Global.Registry.PkgURL,
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

			// Create the form field "package" to match curl
			part, err := fileWriter.CreateFormFile("package", filepath.Base(filePath))
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
			reader, closer := p.Reader(bufferSize, &formData, "nupkg")
			defer closer()

			if len(path) > 0 {
				//This section will get executed only when a nested path is provided via flag
				apiUrlForNestedDirectory := fmt.Sprintf("%s/pkg/%s/%s/nuget/%s", config.Global.Registry.PkgURL, config.Global.AccountID, registryName, path)
				err := uploadNugetPackageDirect(
					context.Background(),
					apiUrlForNestedDirectory,
					fileWriter.FormDataContentType(),
					reader,
					config.Global.AuthToken,
				)
				if err != nil {
					return err
				}

			} else {
				resp, err := pkgClient.UploadNugetPackageWithBodyWithResponse(
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
			}

			progress.Success(fmt.Sprintf("Successfully uploaded package %s", filePath))
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Nested directory")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	cmd.MarkFlagRequired("pkg-url")
	return cmd
}

func uploadNugetPackageDirect(ctx context.Context, url string, contentType string, body io.Reader, apiKey string) error {

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	// Required headers
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-2xx responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"upload failed: status=%d response=%s",
			resp.StatusCode,
			string(bodyBytes),
		)
	}

	return nil
}
