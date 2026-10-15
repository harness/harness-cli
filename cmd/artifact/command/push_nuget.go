package command

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	"github.com/harness/harness-cli/util/common/httpclient"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	NugetFileExtension    = ".nupkg"
	expectedArgumentCount = 2
)

// Must match the registry's NuGet version regex (registry/pkg/nuget/helper.go)
// so metadata targets the version the upload created.
var nugetVersionRegexp = regexp.MustCompile(`^v?([0-9]+(\.[0-9]+)*?)` +
	`(-([0-9]+[0-9A-Za-z\-]*(\.[0-9A-Za-z\-]+)*)|(-([A-Za-z\-]+[0-9A-Za-z\-]*(\.[0-9A-Za-z\-]+)*)))?` +
	`(\+([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?$`)

func NewPushNugetCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	var path string
	var postMetadata string
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
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return cmdutils.ResolvePkgURL(cmd, pkgURL)
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

			// Compute checksums of the file for X-Checksum-* headers
			checksums, err := utils.ComputeFileChecksums(filePath)
			if err != nil {
				progress.Error("Failed to compute file checksums")
				return fmt.Errorf("failed to compute checksums for %s: %w", filePath, err)
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

			if len(path) > 0 {
				//This section will get executed only when a nested path is provided via flag
				apiUrlForNestedDirectory := fmt.Sprintf("%s/pkg/%s/%s/nuget/%s", config.Global.Registry.PkgURL, config.Global.AccountID, registryName, path)
				err := uploadNugetPackageDirect(
					context.Background(),
					apiUrlForNestedDirectory,
					fileWriter.FormDataContentType(),
					&formData,
					config.Global.AuthToken,
					progress,
					bufferSize,
					checksums,
				)
				if err != nil {
					return err
				}

			} else {
				pkgClient := c.PkgHttpClientWithProgress(progress, bufferSize, "nupkg")
				resp, err := pkgClient.UploadNugetPackageWithBodyWithResponse(
					context.Background(),
					config.Global.AccountID,
					registryName,
					fileWriter.FormDataContentType(),
					&formData,
					func(ctx context.Context, req *http.Request) error {
						utils.SetChecksumHeaders(req.Header, checksums)
						return nil
					},
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
			if postMetadata != "" {
				id, version, err := readNugetIdentity(filePath)
				if err != nil {
					logrus.Warnf("Skipping metadata publish for %s: %v", fileName, err)
					return nil
				}
				applyPostPushMetadata(c, postMetadata, registryName, id, version)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Nested directory")
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	cmd.Flags().StringVar(&postMetadata, "metadata", "", "Metadata key-value pairs to attach after push (format: key:value,key2:value2)")

	return cmd
}

func uploadNugetPackageDirect(ctx context.Context, url string, contentType string, body io.Reader, apiKey string, progress p.Reporter, bufferSize int64, checksums utils.FileChecksums) error {

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	// Required headers
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-api-key", apiKey)

	utils.SetChecksumHeaders(req.Header, checksums)

	//creating retry client
	client := httpclient.NewRetryClientWithProgress(progress, bufferSize, "nupkg")

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

type nuspecIdentity struct {
	Metadata struct {
		ID      string `xml:"id"`
		Version string `xml:"version"`
	} `xml:"metadata"`
}

// readNugetIdentity returns the package id and version in the lowercased,
// normalized form the registry stores them under. It selects the .nuspec the
// way the registry's buildMetadata does (registry/pkg/nuget/local.go): every
// entry ending in ".nuspec" (case-sensitive, any directory) is parsed and the
// last one wins.
func readNugetIdentity(nupkgPath string) (string, string, error) {
	zr, err := zip.OpenReader(nupkgPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open package as a zip archive: %w", err)
	}
	defer zr.Close()

	var spec *nuspecIdentity
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".nuspec") {
			continue
		}
		parsed, err := parseNuspecIdentity(f)
		if err != nil {
			return "", "", err
		}
		spec = parsed
	}
	if spec == nil {
		return "", "", fmt.Errorf("no .nuspec found in the package")
	}

	id := strings.TrimSpace(spec.Metadata.ID)
	rawVersion := strings.TrimSpace(spec.Metadata.Version)
	if id == "" || rawVersion == "" {
		return "", "", fmt.Errorf(".nuspec is missing <id> or <version>")
	}
	version, err := normalizeNugetVersion(rawVersion)
	if err != nil {
		return "", "", err
	}
	return strings.ToLower(id), version, nil
}

func parseNuspecIdentity(f *zip.File) (*nuspecIdentity, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
	}
	defer rc.Close()
	var spec nuspecIdentity
	if err := xml.NewDecoder(rc).Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
	}
	return &spec, nil
}

// normalizeNugetVersion mirrors the registry's validateAndNormaliseVersion:
// pad to three numeric segments, keep a non-zero fourth, keep the prerelease
// label, drop build metadata, and lowercase.
func normalizeNugetVersion(v string) (string, error) {
	matches := nugetVersionRegexp.FindStringSubmatch(v)
	if matches == nil {
		return "", fmt.Errorf("malformed nuget version: %s", v)
	}
	parts := strings.Split(matches[1], ".")
	if len(parts) > 4 {
		return "", fmt.Errorf("malformed nuget version: %s", v)
	}
	segments := make([]int64, 0, 4)
	for _, s := range parts {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return "", fmt.Errorf("malformed nuget version %s: %w", v, err)
		}
		segments = append(segments, n)
	}
	for len(segments) < 3 {
		segments = append(segments, 0)
	}

	normalized := fmt.Sprintf("%d.%d.%d", segments[0], segments[1], segments[2])
	if len(segments) > 3 && segments[3] > 0 {
		normalized = fmt.Sprintf("%s.%d", normalized, segments[3])
	}
	normalized += matches[3]
	return strings.ToLower(normalized), nil
}
