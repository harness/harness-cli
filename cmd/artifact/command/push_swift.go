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
	"strings"

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
	SwiftSupportedFileExtension = ".zip"
)

func NewPushSwiftCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	const expectedNumberOfArgument = 3
	cmd := &cobra.Command{
		Use:   "swift  <registry_name> <file_path> <SCOPE>/<NAME>/<VERSION>",
		Short: "Push Swift Artifacts",
		Long:  "Push Swift Artifacts to Harness Artifact Registry",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedNumberOfArgument {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					expectedNumberOfArgument, len(args), cmd.UseLine(),
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
			targetPackagePath := args[2]

			// Create progress reporter
			progress := p.NewConsoleReporter()

			// Validate Registry Name and file_path
			progress.Start("Validating input parameters")

			// Resolve file path
			/*files, err := utils.ResolveFilePath(filePath, SwiftSupportedFileExtension)
			if err != nil {
				progress.Error("Failed to resolve file path")
				return err
			}

			*/
			//filePath = files[0]

			fileName := filepath.Base(filePath)

			// validate file name
			valid, err := fileutil.IsFilenameAcceptable(fileName, SwiftSupportedFileExtension)
			if !valid {
				progress.Error("Invalid file name")
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to validate package file name: %v", err))
			}

			// Validate file exists
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
			}
			if fileInfo.IsDir() {
				return errors.NewValidationError("file_path", "package file path must be a file, not a directory")
			}

			taregetDetail, err := parseTargetPath(targetPackagePath)
			if err != nil {
				progress.Error("Failed to validate input parameter")
				return err
			}
			targetScope := taregetDetail[0]
			packageName := taregetDetail[1]
			version := taregetDetail[2]

			tempRes := fmt.Sprintf("target detail %s %s %s", targetScope, packageName, version)
			fmt.Println(tempRes)
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
			reader, closer := p.Reader(bufferSize, &formData, "swift")
			defer closer()

			//TODO may be creation of custome header is required like conda
			//and pass that reader

			resp, err := pkgClient.UploadSwiftPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				targetScope,
				packageName,
				version,
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

// parsing input of the form ,based on first and last slash
// <SCOPE>/<NAME>/<VERSION>
func parseTargetPath(input string) ([]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("input cannot be empty")
	}

	parts := strings.Split(input, "/")

	// Must have exactly 3 parts: <SCOPE>/<NAME>/<VERSION>
	if len(parts) != 3 {
		return nil, fmt.Errorf(
			"invalid format Must have exactly 3 parts :: found %d : expected '<SCOPE>/<NAME>/<VERSION>'",
			len(parts),
		)
	}

	scope := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	version := strings.TrimSpace(parts[2])

	// Validate each part
	if scope == "" {
		return nil, fmt.Errorf("invalid format: scope is empty")
	}
	if name == "" {
		return nil, fmt.Errorf("invalid format: name is empty")
	}
	if version == "" {
		return nil, fmt.Errorf("invalid format: version is empty")
	}

	return []string{scope, name, version}, nil
}
