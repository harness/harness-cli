package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"harness/cmd/common/auth"
	"harness/cmd/common/printer"
	"harness/config"
	client "harness/internal/api/ar"
	pkgclient "harness/internal/api/ar_pkg"
)

// NewPushGenericCmd creates a new cobra.Command for pushing generic artifacts to the registry.
// command example: hns ar push generic <registry_name> <package_file_path>
func NewPushGenericCmd(c *client.ClientWithResponses) *cobra.Command {
	var packageName, filename, packageVersion, description string
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "generic <registry_name> <package_file_path>",
		Short: "Push Generic Artifacts",
		Long:  "Push Generic Artifacts to Harness Artifact Registry",
		Args:  cobra.ExactArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			config.Global.Registry.PkgURL = pkgURL
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			packageFilePath := args[1]

			// Validate file exists
			fileInfo, err := os.Stat(packageFilePath)
			if err != nil {
				return fmt.Errorf("failed to access package file: %w", err)
			}
			if fileInfo.IsDir() {
				return errors.New("package file path must be a file, not a directory")
			}

			// If version is not provided, use a default version
			version := packageVersion
			if version == "" {
				version = "1.0.0"
			}

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetXApiKeyOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			// Create a buffer to store the multipart form data
			var buffer bytes.Buffer
			writer := multipart.NewWriter(&buffer)

			// Read file content
			fileContent, err := ioutil.ReadFile(packageFilePath)
			if err != nil {
				return fmt.Errorf("failed to read package file: %w", err)
			}

			// If package name is not provided, use the filename without extension
			if filename == "" {
				filename = filepath.Base(packageFilePath)
				ext := filepath.Ext(filename)
				if ext != "" {
					filename = filename[:len(filename)-len(ext)]
				}
			}

			filePart, err := writer.CreateFormFile("file", filename)
			if err != nil {
				return fmt.Errorf("failed to create form file: %w", err)
			}

			_, err = filePart.Write(fileContent)
			if err != nil {
				return fmt.Errorf("failed to write file content: %w", err)
			}

			// Add filename field
			err = writer.WriteField("filename", filename)
			if err != nil {
				return fmt.Errorf("failed to write filename field: %w", err)
			}

			// Add description field if provided
			if description != "" {
				err = writer.WriteField("description", description)
				if err != nil {
					return fmt.Errorf("failed to write description field: %w", err)
				}
			}

			// Close the writer to set the terminating boundary
			err = writer.Close()
			if err != nil {
				return fmt.Errorf("failed to close multipart writer: %w", err)
			}

			fmt.Printf("Uploading generic package '%s' (version '%s', filename '%s', description '%s') to registry '%s'...\n",
				packageName, version, filename, description, registryName)
			
			// Call the API with the proper parameters and multipart form body
			response, err := pkgClient.UploadGenericPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,      // accountId
				registryName,                 // registry
				packageName,                  // package name
				version,                      // version
				writer.FormDataContentType(), // content type for multipart/form-data
				&buffer,                      // multipart form data as io.Reader
			)
			if err != nil {
				return fmt.Errorf("failed to upload package: %w", err)
			}

			// Check response status
			if response.StatusCode() >= 400 {
				return fmt.Errorf("server returned error: %s", response.Status())
			}

			// Print success with artifact details
			fmt.Printf("Successfully uploaded generic package '%s' (version '%s') to registry '%s'\n",
				packageName, version, registryName)

			// Use default json options for response printing
			if response.Body != nil {
				options := printer.DefaultJsonOptions()
				printer.PrintJsonWithOptions(response.Body, options)
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&packageName, "name", "", "name for the artifact")
	cmd.Flags().StringVar(&filename, "filename", "",
		"name of the file being uploaded (defaults to filename without extension)")
	cmd.Flags().StringVar(&packageVersion, "version", "", "version for the artifact (defaults to '1.0.0')")
	cmd.Flags().StringVar(&description, "description", "", "description of the artifact")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")

	cmd.MarkFlagRequired("pkg-url")
	cmd.MarkFlagRequired("name")

	return cmd
}
