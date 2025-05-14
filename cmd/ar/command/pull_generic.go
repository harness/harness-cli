package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"harness/config"
	client "harness/internal/api/ar"
	pkgclient "harness/internal/api/ar_pkg"
	"harness/util/common"
	"harness/util/common/auth"
	"harness/util/common/printer"
	"harness/util/common/progress"
)

func printReadCloser(rc io.ReadCloser) {
	defer rc.Close() // always close when done reading

	// Read all data
	data, err := io.ReadAll(rc)
	if err != nil {
		log.Fatalf("error reading: %v", err)
	}

	fmt.Println(string(data))
}

// NewPullGenericCmd creates a new cobra.Command for pulling generic artifacts from the registry.
// command example: hns ar pull generic <registry_name> <package_path> <destination_path>
func NewPullGenericCmd(c *client.ClientWithResponses) *cobra.Command {
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "generic <registry_name> <package_path> <destination_path>",
		Short: "Pull Generic Artifacts",
		Long:  "Pull Generic Artifacts from Harness Artifact Registry",
		Args:  cobra.ExactArgs(3),
		PreRun: func(cmd *cobra.Command, args []string) {
			config.Global.Registry.PkgURL = pkgURL
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			packagePath := args[1]
			destinationPath := args[2]

			// Parse package path: <package_name>/<version>/<filename>
			splits := strings.SplitN(packagePath, "/", 3)
			if len(splits) != 3 {
				return fmt.Errorf("invalid package path format: %s (expected format: <package_name>/<version>/<filename>)",
					packagePath)
			}

			packageName := splits[0]
			packageVersion := splits[1]
			packageFile := splits[2]

			// Prepare download parameters
			params := &pkgclient.DownloadGenericPackageParams{
				Filename: packageFile,
			}

			// Initialize the package client
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetXApiKeyOptionARPKG())
			if err != nil {
				return fmt.Errorf("failed to create package client: %w", err)
			}

			fmt.Printf("Pulling generic package '%s' (version '%s', file: '%s') from registry '%s'...\n",
				packageName, packageVersion, packageFile, registryName)

			// Download the package
			response, err := pkgClient.DownloadGenericPackage(
				context.Background(),
				config.Global.AccountID,
				registryName,
				packageName,
				packageVersion,
				params)
			if err != nil {
				return fmt.Errorf("failed to download package: %w", err)
			}

			fmt.Println(response.Header.Get("Content-Disposition"))

			// Check response status
			if response.StatusCode >= 400 {
				return fmt.Errorf("server returned error: %s", response.Status)
			}

			// If the response is empty, return an error
			if response.Body == nil {
				return errors.New("received empty response from server")
			}

			// Determine file name from Content-Disposition header or use default
			saveFilename := packageFile
			// Try to get filename from Content-Disposition header
			if disposition := response.Header.Get("Content-Disposition"); disposition != "" {
				parts := strings.Split(disposition, "filename=")
				if len(parts) > 1 {
					saveFilename = strings.Trim(parts[1], `"'\r\n `)
				}
			}

			// Ensure destination directory exists
			destDir := destinationPath
			fileInfo, err := os.Stat(destinationPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("failed to access destination path: %w", err)
				}
				// Create directory if it doesn't exist
				if err := os.MkdirAll(destinationPath, 0755); err != nil {
					return fmt.Errorf("failed to create destination directory: %w", err)
				}
			} else if !fileInfo.IsDir() {
				// If destination exists but is not a directory, use it as the file path
				destDir = filepath.Dir(destinationPath)
				saveFilename = filepath.Base(destinationPath)
			}

			// Construct the final file path
			filePath := filepath.Join(destDir, saveFilename)
			if fileInfo != nil && fileInfo.IsDir() {
				filePath = filepath.Join(destinationPath, saveFilename)
			}

			// Create the destination file
			outFile, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create destination file: %w", err)
			}
			defer outFile.Close()
			log.Printf("length: %d, size: %d, savefilename: %s\n", response.ContentLength, response.ContentLength,
				saveFilename)

			reader, closer := progress.Reader(response.ContentLength, response.Body, saveFilename)
			defer closer()
			written, err := io.Copy(outFile, reader)

			if err != nil {
				return fmt.Errorf("failed to write to destination file: %w", err)
			}

			fmt.Printf("Successfully downloaded generic package '%s' (version '%s') from registry '%s'\n",
				packageName, packageVersion, registryName)
			fmt.Printf("Saved to %s (%d bytes)\n", filePath, written)

			// Print success message - no JSON metadata is returned for download
			options := printer.DefaultJsonOptions()
			options.ShowPagination = false
			metadata := map[string]interface{}{
				"registry": registryName,
				"package":  packageName,
				"version":  packageVersion,
				"filename": saveFilename,
				"size":     common.GetSize(written),
				"path":     filePath,
			}
			printer.PrintJsonWithOptions(metadata, options)

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	cmd.MarkFlagRequired("pkg-url")

	return cmd
}
