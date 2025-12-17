package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/module/ar/migrate/types/npm"
	"github.com/harness/harness-cli/util/common/auth"
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
			config.Global.Registry.PkgURL = pkgURL
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
			progress.Success("Input parameters validated")

			// Extract package.json from tarball
			progress.Step("Extracting package.json from tarball")
			pkgJSONBytes, err := extractPackageJSONFromTarball(packageFilePath)
			if err != nil {
				progress.Error("Failed to extract package.json from tarball")
				return fmt.Errorf("failed to extract package.json from tarball: %w", err)
			}

			// Build NPM upload payload
			progress.Step("Building NPM upload payload")
			upload, pkgName, version, err := buildNpmUploadFromPackageJSON(pkgJSONBytes, packageFilePath)
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
			pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
				auth.GetXApiKeyOptionARPKG())
			if err != nil {
				progress.Error("Failed to create package client")
				return fmt.Errorf("failed to create package client: %w", err)
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
			reader, closer := p.Reader(bufferSize, pr, fileInfo.Name())
			defer closer()

			resp, err := pkgClient.UploadNPMPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				pkgName,
				"application/json",
				reader,
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
	cmd.MarkFlagRequired("pkg-url")

	return cmd
}

func extractPackageJSONFromTarball(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open tarball: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.FileInfo().IsDir() {
			continue
		}

		base := filepath.Base(header.Name)
		if base == "package.json" {
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read package.json from tarball: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("package.json not found in tarball")
}

// minimalPackageJSON represents the subset of fields from package.json we care about.
type minimalPackageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	Homepage             string            `json:"homepage"`
	Keywords             []string          `json:"keywords"`
	Repository           interface{}       `json:"repository"`
	Author               interface{}       `json:"author"`
	License              interface{}       `json:"license"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Bin                  interface{}       `json:"bin"`
}

func buildNpmUploadFromPackageJSON(pkgJSON []byte, tarballPath string) (*npm.PackageUpload, string, string, error) {
	var pkg minimalPackageJSON
	if err := json.Unmarshal(pkgJSON, &pkg); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse package.json: %w", err)
	}

	if pkg.Name == "" || pkg.Version == "" {
		return nil, "", "", fmt.Errorf("package.json must contain 'name' and 'version'")
	}

	versionObj := &npm.PackageMetadataVersion{
		ID:                   pkg.Name + "@" + pkg.Version,
		Name:                 pkg.Name,
		Version:              pkg.Version,
		Description:          pkg.Description,
		Author:               pkg.Author,
		Homepage:             pkg.Homepage,
		License:              pkg.License,
		Repository:           pkg.Repository,
		Keywords:             pkg.Keywords,
		Dependencies:         pkg.Dependencies,
		BundleDependencies:   nil,
		DevDependencies:      pkg.DevDependencies,
		PeerDependencies:     pkg.PeerDependencies,
		Bin:                  pkg.Bin,
		OptionalDependencies: pkg.OptionalDependencies,
		Readme:               "",
		Dist:                 npm.PackageDistribution{},
		Maintainers:          nil,
	}

	metadata := npm.PackageMetadata{
		ID:          pkg.Name,
		Name:        pkg.Name,
		Description: pkg.Description,
		DistTags: map[string]string{
			"latest": pkg.Version,
		},
		Versions: map[string]*npm.PackageMetadataVersion{
			pkg.Version: versionObj,
		},
		Readme:         "",
		Maintainers:    nil,
		Time:           nil,
		Homepage:       pkg.Homepage,
		Keywords:       pkg.Keywords,
		Repository:     pkg.Repository,
		Author:         pkg.Author,
		ReadmeFilename: "",
		Users:          nil,
		License:        pkg.License,
	}

	// Read tarball and base64 encode it into _attachments
	file, err := os.Open(tarballPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to open tarball for attachment: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read tarball for attachment: %w", err)
	}

	b64Data := base64.StdEncoding.EncodeToString(data)

	tarballName := filepath.Base(tarballPath)

	upload := &npm.PackageUpload{
		PackageMetadata: metadata,
		Attachments: map[string]*npm.PackageAttachment{
			tarballName: {
				ContentType: "application/octet-stream",
				Data:        b64Data,
				Length:      len(data),
			},
		},
	}

	return upload, pkg.Name, pkg.Version, nil
}
