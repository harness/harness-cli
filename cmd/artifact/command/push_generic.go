package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

// NewPushGenericCmd creates a new cobra.Command for pushing generic artifacts to the registry.
// command example: hc ar push generic <registry_name> <package_file_path>
func NewPushGenericCmd(c *cmdutils.Factory) *cobra.Command {
	var packageName, filename, packageVersion, description, path string
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

			if filename == "" {
				filename = filepath.Base(packageFilePath)
			}

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetXApiKeyOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			if path == "" {
				path = filename
			}
			// Use file upload API when path is specified
			fmt.Printf("Uploading generic file '%s' to path '%s' (version '%s') in registry '%s'...\n",
				filename, path, version, registryName)

			// Read file content directly for file upload
			file, err := os.Open(packageFilePath)
			if err != nil {
				return fmt.Errorf("failed to open package file: %w", err)
			}
			defer file.Close()

			bufferSize := int64(fileInfo.Size())
			reader, closer := progress.Reader(bufferSize, file, filename)
			defer closer()

			response, err := pkgClient.UploadGenericFileToPathWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,    // accountId
				registryName,               // registry
				packageName,                // package name
				version,                    // version
				path,                       // filepath
				"application/octet-stream", // content type
				reader,                     // file content as io.Reader with progress tracking
			)
			if err != nil {
				return fmt.Errorf("failed to upload file: %w", err)
			}

			// Check response status
			if response.StatusCode() >= 400 {
				return fmt.Errorf("server returned error: %s", response.Status())
			}

			fmt.Printf("Successfully uploaded generic file '%s' to path '%s' (version '%s') in registry '%s'\n",
				filename, path, version, registryName)

			// Use default json options for response printing
			if response.Body != nil {
				options := printer.DefaultJsonOptions()
				options.ShowPagination = false
				printer.PrintJsonWithOptions(response.Body, options)
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&packageName, "name", "n", "", "name for the artifact")
	cmd.Flags().StringVar(&filename, "filename", "",
		"name of the file being uploaded (defaults to name of the file being uploaded)")
	cmd.Flags().StringVar(&packageVersion, "version", "", "version for the artifact (defaults to '1.0.0')")
	cmd.Flags().StringVarP(&description, "description", "d", "", "description of the artifact (default to empty)")
	cmd.Flags().StringVar(&path, "path", "", "file path within the package (if not specified, uses file name as file path)")

	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")

	cmd.MarkFlagRequired("pkg-url")
	cmd.MarkFlagRequired("name")

	return cmd
}
