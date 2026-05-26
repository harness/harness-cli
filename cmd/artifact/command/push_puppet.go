package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

const (
	puppetTarGzExt    = ".tar.gz"
	puppetTgzExt      = ".tgz"
	puppetMetadataKey = "metadata.json"
	// 1GB cap on tar reads to bound memory while scanning for metadata.json.
	puppetTarReadLimit = 1 << 30
)

// puppetMetadata is read from the tarball only to display the module
// name@version in CLI progress output. The server re-parses the tarball
// and is the source of truth for all validation.
type puppetMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// NewPushPuppetCmd creates a new cobra.Command for pushing Puppet modules.
// Command example: hc artifact push puppet <registry_name> <module_tar_gz_path>
func NewPushPuppetCmd(c *cmdutils.Factory) *cobra.Command {
	const expectedNumberOfArgument = 2
	cmd := &cobra.Command{
		Use:   "puppet <registry_name> <module_tar_gz_path>",
		Short: "Push Puppet module",
		Long:  "Push a Puppet module .tar.gz tarball to Harness Artifact Registry (HAR)",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != expectedNumberOfArgument {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					expectedNumberOfArgument, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			packageFilePath := args[1]

			progress := p.NewConsoleReporter()

			progress.Start("Validating input parameters")

			files, err := utils.ResolveFilePath(packageFilePath, puppetTarGzExt, puppetTgzExt, ".gz")
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
			if !isPuppetTarball(packageFilePath) {
				progress.Error(fmt.Sprintf("Package file must be a %s or %s tarball", puppetTarGzExt, puppetTgzExt))
				return fmt.Errorf("package file must be a %s or %s tarball, got: %s", puppetTarGzExt, puppetTgzExt, filepath.Ext(packageFilePath))
			}
			progress.Success("Input parameters validated")

			progress.Step(fmt.Sprintf("Extracting %s from tarball", puppetMetadataKey))
			metadata, err := extractPuppetMetadata(packageFilePath)
			if err != nil {
				progress.Error(fmt.Sprintf("Failed to extract %s from tarball", puppetMetadataKey))
				return fmt.Errorf("failed to extract %s from tarball: %w", puppetMetadataKey, err)
			}
			if metadata.Name == "" || metadata.Version == "" {
				progress.Error(fmt.Sprintf("%s must contain non-empty 'name' and 'version'", puppetMetadataKey))
				return fmt.Errorf("%s must contain non-empty 'name' and 'version'", puppetMetadataKey)
			}
			progress.Success(fmt.Sprintf("Module metadata extracted: %s@%s", metadata.Name, metadata.Version))

			// Initialize the package client
			pkgClient := c.PkgHttpClient()

			file, err := os.Open(packageFilePath)
			if err != nil {
				progress.Error("Failed to open package file")
				return fmt.Errorf("failed to open package file: %w", err)
			}
			defer file.Close()

			pr, pw := io.Pipe()
			writer := multipart.NewWriter(pw)
			go func() {
				defer pw.Close()
				defer writer.Close()
				part, err := writer.CreateFormFile("file", filepath.Base(packageFilePath))
				if err != nil {
					pw.CloseWithError(fmt.Errorf("failed to create form file: %w", err))
					return
				}
				if _, err := io.Copy(part, file); err != nil {
					pw.CloseWithError(fmt.Errorf("failed to copy file to form: %w", err))
					return
				}
			}()

			progress.Step("Uploading module to registry")
			reader, closer := p.Reader(fileInfo.Size(), pr, fileInfo.Name())
			defer closer()

			resp, err := pkgClient.UploadPuppetPackageWithBodyWithResponse(
				context.Background(),
				config.Global.AccountID,
				registryName,
				writer.FormDataContentType(),
				reader,
			)
			if err != nil {
				progress.Error("Failed to upload Puppet module")
				return fmt.Errorf("failed to upload Puppet module: %w", err)
			}

			if resp.StatusCode() != http.StatusOK &&
				resp.StatusCode() != http.StatusCreated &&
				resp.StatusCode() != http.StatusNoContent {
				progress.Error("Upload failed")
				return fmt.Errorf("failed to upload Puppet module: %s\nresponse: %s", resp.Status(), resp.Body)
			}

			progress.Success(fmt.Sprintf(
				"Successfully uploaded Puppet module '%s@%s' to registry '%s'",
				metadata.Name, metadata.Version, registryName,
			))
			return nil
		},
	}

	return cmd
}

// isPuppetTarball returns true if path ends with .tar.gz or .tgz.
func isPuppetTarball(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, puppetTarGzExt) || strings.HasSuffix(lower, puppetTgzExt)
}

// extractPuppetMetadata locates and parses the top-level metadata.json inside a
// Puppet module .tar.gz. Puppet modules are packaged as "<owner>-<name>-<version>/metadata.json"
// so the file lives one directory level deep.
func extractPuppetMetadata(path string) (*puppetMetadata, error) {
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

	tarReader := tar.NewReader(io.LimitReader(gzReader, puppetTarReadLimit))
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
		if filepath.Base(header.Name) != puppetMetadataKey {
			continue
		}
		// Only accept metadata.json at the top-level module directory
		// (one separator after the leading directory). Nested matches
		// (e.g. tests/fixtures/.../metadata.json) are skipped.
		if strings.Count(strings.Trim(header.Name, "/"), "/") != 1 {
			continue
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", puppetMetadataKey, err)
		}
		var meta puppetMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", puppetMetadataKey, err)
		}
		return &meta, nil
	}
	return nil, fmt.Errorf("%s not found at top level of tarball", puppetMetadataKey)
}
