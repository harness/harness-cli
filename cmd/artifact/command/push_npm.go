package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

// NewPushNpmCmd creates a new cobra.Command for pushing NPM packages.
// Command example: hc artifact push npm <registry_name> <npm_tgz_path>
func NewPushNpmCmd(f *cmdutils.Factory) *cobra.Command {
	var pkgURL string

	cmd := &cobra.Command{
		Use:   "npm <registry_name> <npm_tgz_path>",
		Short: "Push NPM package",
		Long:  "Push an NPM .tgz package to Harness Artifact Registry (HAR)",
		Args:  cobra.ExactArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			if pkgURL != "" {
				config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURL)
			}
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

			// Resolve file path (supports glob patterns like *.tgz)
			files, err := utils.ResolveFilePath(packageFilePath, ".tgz")
			if err != nil {
				progress.Error("Failed to resolve file path")
				return err
			}
			packageFilePath = files[0]
			progress.Step(fmt.Sprintf("Uploading file: %s", packageFilePath))

			fileInfo, err := os.Stat(packageFilePath)
			if err != nil {
				progress.Error("Failed to access package file")
				return fmt.Errorf("failed to access package file: %w", err)
			}
			if fileInfo.IsDir() {
				progress.Error("Package file path must be a file, not a directory")
				return errors.New("package file path must be a file, not a directory")
			}

			if !(filepath.Ext(packageFilePath) == ".tgz" || filepath.Ext(packageFilePath) == ".gz" || filepath.Ext(packageFilePath) == ".tgz") {
				// Allow .tgz or .tar.gz; simple extension check
				// More robust checks can be added later if needed
			}
			// Compute checksums of the file for X-Checksum-* headers
			checksums, err := utils.ComputeFileChecksums(packageFilePath)
			if err != nil {
				progress.Error("Failed to compute file checksums")
				return fmt.Errorf("failed to compute checksums for %s: %w", packageFilePath, err)
			}

			progress.Success("Input parameters validated")

			// Extract package.json from tarball
			progress.Step("Extracting package.json from tarball")
			file, err := os.Open(packageFilePath)
			if err != nil {
				progress.Error("Failed to open tarball")
				return fmt.Errorf("failed to open tarball: %w", err)
			}
			defer file.Close()

			pkgJSONBytes, readme, err := utils.ExtractPackageJSONAndReadmeFromTarball(file)
			if err != nil {
				progress.Error("Failed to extract package.json from tarball")
				return fmt.Errorf("failed to extract package.json from tarball: %w", err)
			}

			// Build NPM upload payload
			file, err = os.Open(packageFilePath)
			if err != nil {
				progress.Error("Failed to open tarball")
				return fmt.Errorf("failed to open tarball: %w", err)
			}
			defer file.Close()

			progress.Step("Building NPM upload payload")
			upload, pkgName, version, err := utils.BuildNpmUploadFromPackageJSON(pkgJSONBytes, readme, file)
			if err != nil {
				progress.Error("Failed to build NPM upload body")
				return fmt.Errorf("failed to build NPM upload body: %w", err)
			}

			if pkgName == "" || version == "" {
				progress.Error("Package.json must contain non-empty 'name' and 'version'")
				return fmt.Errorf("package.json must contain non-empty 'name' and 'version'")
			}

			if config.Global.Registry.PkgURL == "" {
				progress.Error("pkg-url must be set")
				return fmt.Errorf("pkg-url must be set")
			}
			progress.Success(fmt.Sprintf("Package metadata extracted: %s@%s", pkgName, version))

			// Initialize the package client
			progress.Step("Initializing package client")
			pkgClient := f.PkgHttpClient()

			progress.Step("checking if already exist")
			//calling to get all the existing version to prevent duplicate upload ,same as npm publish
			metadataResp, err := pkgClient.DownloadNPMPackageMetadataWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				pkgName,
			)
			if err != nil {
				progress.Error("Failed to download NPM package detail ")
				return fmt.Errorf("Failed to  download NPM package details: %w", err)
			}
			// Check response
			if metadataResp.StatusCode() != http.StatusOK && metadataResp.StatusCode() != http.StatusNotFound {
				progress.Error("download of metadata  failed")
				status := ""
				var body []byte
				if metadataResp != nil {
					status = metadataResp.Status()
					body = metadataResp.Body
				}
				return fmt.Errorf("failed to download NPM metadata: %s \n response: %s", status, body)
			}
			//Check for pre exist only if success response came
			if metadataResp.StatusCode() == http.StatusOK {

				var existingPkgDetails NpmPackage
				if err := json.Unmarshal(metadataResp.Body, &existingPkgDetails); err != nil {
					return err
				}

				if err != nil {
					return fmt.Errorf("failed to parse response data %w", err)
				}

				if _, ok := existingPkgDetails.Versions[version]; ok {
					progress.Error(fmt.Sprintf("You cannot publish over the previously published versions %s", version))
					return fmt.Errorf("already exist %s", version)
				}
			}

			// Prepare streaming JSON body from PackageUpload
			progress.Step("Preparing package upload")
			pr, pw := io.Pipe()
			encoder := json.NewEncoder(pw)
			encoder.SetEscapeHTML(false)

			go func() {
				defer pw.Close()
				if err := encoder.Encode(upload); err != nil {
					pw.CloseWithError(fmt.Errorf("failed to encode upload JSON: %w", err))
				}
			}()

			// Upload package using generated client
			progress.Step("Uploading package to registry")

			// Initialize progress reader for upload tracking
			bufferSize := fileInfo.Size()

			//Re-initializing pkgClient with progress reader for upload tracking
			pkgClient = f.PkgHttpClientWithProgress(progress, bufferSize, fileInfo.Name())

			resp, err := pkgClient.UploadNPMPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				pkgName,
				"application/json",
				pr,
				func(ctx context.Context, req *http.Request) error {
					utils.SetChecksumHeaders(req.Header, checksums)
					return nil
				},
			)
			if err != nil {
				progress.Error("Failed to upload NPM package")
				return fmt.Errorf("failed to upload NPM package: %w", err)
			}

			// Check response
			if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
				progress.Error("Upload failed")
				return fmt.Errorf("failed to upload NPM package: %s \n response: %s", resp.Status(), resp.Body)
			}

			progress.Success(fmt.Sprintf("Successfully uploaded NPM package '%s@%s' to registry '%s'", pkgName, version, registryName))
			return nil
		},
	}

	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages service")

	return cmd
}

type NpmPackage struct {
	ID       string                `json:"_id"`
	Name     string                `json:"name"`
	Versions map[string]NpmVersion `json:"versions"`
}

type NpmVersion struct {
	Version string `json:"version"`
}
