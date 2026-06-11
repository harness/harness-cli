package command

import (
	"bufio"
	"bytes"
	"context"
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
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/artifact"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

const (
	DebFileExtension               = ".deb"
	DscFileExtension               = ".dsc"
	debianExpectedNumberOfArgument = 2
)

// DscMetadata contains parsed information from a .dsc file
type DscMetadata struct {
	Source  string
	Version string
}

func NewPushDebianCmd(c *cmdutils.Factory) *cobra.Command {
	var distribution string
	var component string
	var sourceFile string
	var originSourceFile string

	cmd := &cobra.Command{
		Use:   "debian <registry_name> <file_path>",
		Short: "Push Debian Artifacts",
		Long:  "Push Debian packages (.deb) or source packages (.dsc) to Harness Artifact Registry, when uploading .dsc file, you need to provide --source-file and --origin-source-file",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != debianExpectedNumberOfArgument {
				return fmt.Errorf(
					"Error: Invalid number of argument,  accepts %d arg(s), received %d  \nUsage :\n %s",
					debianExpectedNumberOfArgument, len(args), cmd.UseLine(),
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registryName := args[0]
			filePath := args[1]

			// Create progress reporter
			progress := p.NewConsoleReporter()

			// Determine file type based on extension
			fileExt := filepath.Ext(filePath)

			switch fileExt {
			case DebFileExtension:
				// Handle .deb package
				return handleDebPackage(registryName, filePath, distribution, component, progress)
			case DscFileExtension:
				// Handle .dsc source package
				return handleDebSourcePackage(registryName, filePath, distribution, component, sourceFile, originSourceFile, progress)
			default:
				progress.Error("Unsupported file type")
				return errors.NewValidationError("file_path", fmt.Sprintf("file must be either .deb or .dsc, got: %s", fileExt))
			}
		},
	}

	cmd.Flags().StringVar(&distribution, "distribution", "", "Debian distribution name (e.g., focal, bullseye)")
	cmd.Flags().StringVar(&component, "component", "", "Debian component name (e.g., main, contrib, non-free)")
	cmd.Flags().StringVar(&sourceFile, "source-file", "", "Path to source file (only for .dsc files)")
	cmd.Flags().StringVar(&originSourceFile, "origin-source-file", "", "Path to origin source file (only for .dsc files)")
	cmd.MarkFlagRequired("distribution")
	cmd.MarkFlagRequired("component")

	return cmd
}

// handleDebPackage handles uploading .deb packages
func handleDebPackage(registryName, filePath, distribution, component string, progress *p.ConsoleReporter) error {
	// Resolve file path (supports glob patterns like *.deb)
	files, err := utils.ResolveFilePath(filePath, DebFileExtension)
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
	valid, err := fileutil.IsFilenameAcceptable(fileName, DebFileExtension)
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

	// Create the form field "file" to match API expectations
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
	reader, closer := p.Reader(bufferSize, &formData, "debian")
	defer closer()

	// Build query parameters
	params := &pkgclient.UploadDebianDebFileParams{
		Distribution: distribution,
		Component:    component,
	}

	resp, err := pkgClient.UploadDebianDebFileWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		params,
		fileWriter.FormDataContentType(),
		reader,
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

	progress.Success(fmt.Sprintf("Successfully uploaded package %s", filePath))
	return nil
}

// handleDebSourcePackage handles uploading .dsc source packages
func handleDebSourcePackage(registryName, dscFilePath, distribution, component, sourceFile, originSourceFile string, progress *p.ConsoleReporter) error {
	// Validate at least one source file is provided
	if sourceFile == "" && originSourceFile == "" {
		progress.Error("At least one source file is required")
		return errors.NewValidationError("source_files", "at least one of --source-file or --origin-source-file is required for .dsc files")
	}

	// Validate DSC file exists
	dscFileName := filepath.Base(dscFilePath)
	fileInfo, err := os.Stat(dscFilePath)
	if err != nil {
		return errors.NewValidationError("dsc_file_path", fmt.Sprintf("failed to access dsc file: %v", err))
	}
	if fileInfo.IsDir() {
		return errors.NewValidationError("dsc_file_path", "dsc file path must be a file, not a directory")
	}

	// Validate dsc file name
	valid, err := fileutil.IsFilenameAcceptable(dscFileName, DscFileExtension)
	if !valid {
		progress.Error("Invalid DSC file name")
		return errors.NewValidationError("dsc_file_path", fmt.Sprintf("failed to validate dsc file name: %v", err))
	}

	progress.Success("Input parameters validated")

	// Parse DSC file to extract package name and version
	progress.Step("Parsing DSC file metadata")
	dscMetadata, err := parseDscFile(dscFilePath)
	if err != nil {
		progress.Error("Failed to parse DSC file")
		return fmt.Errorf("failed to parse dsc file: %w", err)
	}
	progress.Success(fmt.Sprintf("Extracted package: %s, version: %s", dscMetadata.Source, dscMetadata.Version))

	// Initialize the package client
	pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		return fmt.Errorf("failed to create package client: %w", err)
	}

	// Upload DSC file first
	progress.Step(fmt.Sprintf("Uploading: %s", dscFilePath))
	if err := uploadDscFile(pkgClient, registryName, dscFilePath, distribution, component, progress); err != nil {
		return err
	}

	// Upload tar.xz file if provided
	if sourceFile != "" {
		progress.Step(fmt.Sprintf("Uploading: %s", sourceFile))
		if err := uploadSourceFile(pkgClient, registryName, sourceFile, dscMetadata.Source, dscMetadata.Version, distribution, component, progress, false); err != nil {
			return err
		}
	}

	// Upload originSourceFile if provided
	if originSourceFile != "" {
		progress.Step(fmt.Sprintf("Uploading: %s", originSourceFile))
		upstreamVersion := artifact.ExtractUpstreamVersion(dscMetadata.Version)
		if err := uploadSourceFile(pkgClient, registryName, originSourceFile, dscMetadata.Source, upstreamVersion, distribution, component, progress, true); err != nil {
			return err
		}
	}

	progress.Success("Successfully uploaded all source files")
	return nil
}

// parseDscFile extracts Source and Version fields from a .dsc file
func parseDscFile(filePath string) (*DscMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open dsc file: %w", err)
	}
	defer file.Close()

	metadata := &DscMetadata{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Source:") {
			metadata.Source = strings.TrimSpace(strings.TrimPrefix(line, "Source:"))
		} else if strings.HasPrefix(line, "Version:") {
			metadata.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		}

		// Stop if we have both fields
		if metadata.Source != "" && metadata.Version != "" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading dsc file: %w", err)
	}

	if metadata.Source == "" {
		return nil, fmt.Errorf("Source field not found in dsc file")
	}
	if metadata.Version == "" {
		return nil, fmt.Errorf("Version field not found in dsc file")
	}

	return metadata, nil
}

// uploadDscFile uploads a .dsc file
func uploadDscFile(client *pkgclient.ClientWithResponses, registryName, filePath, distribution, component string, progress *p.ConsoleReporter) error {
	// Compute checksums
	checksums, err := utils.ComputeFileChecksums(filePath)
	if err != nil {
		progress.Error("Failed to compute file checksums")
		return fmt.Errorf("failed to compute checksums for %s: %w", filePath, err)
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		progress.Error("Failed to open dsc file")
		return err
	}
	defer file.Close()

	var formData bytes.Buffer
	fileWriter := multipart.NewWriter(&formData)

	// Create the form field "file" to match API expectations
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
	bufferSize := int64(formData.Len())
	reader, closer := p.Reader(bufferSize, &formData, "dsc")
	defer closer()

	// Build query parameters
	params := &pkgclient.UploadDebianDscFileParams{
		Distribution: distribution,
		Component:    component,
	}

	resp, err := client.UploadDebianDscFileWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		params,
		fileWriter.FormDataContentType(),
		reader,
		func(ctx context.Context, req *http.Request) error {
			utils.SetChecksumHeaders(req.Header, checksums)
			return nil
		},
	)

	if err != nil {
		progress.Error("Failed to upload dsc file")
		return err
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to push dsc file: %s \n response: %s", resp.Status(), resp.Body)
	}

	progress.Success(fmt.Sprintf("Uploaded %s", filepath.Base(filePath)))
	return nil
}

// uploadSourceFile uploads a source file
func uploadSourceFile(client *pkgclient.ClientWithResponses, registryName, filePath, packageName, version, distribution, component string, progress *p.ConsoleReporter, isOrig bool) error {
	// Validate file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return errors.NewValidationError("source_file", fmt.Sprintf("failed to access source file: %v", err))
	}
	if fileInfo.IsDir() {
		return errors.NewValidationError("source_file", "source file path must be a file, not a directory")
	}

	// Compute checksums
	checksums, err := utils.ComputeFileChecksums(filePath)
	if err != nil {
		progress.Error("Failed to compute file checksums")
		return fmt.Errorf("failed to compute checksums for %s: %w", filePath, err)
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		progress.Error("Failed to open source file")
		return err
	}
	defer file.Close()

	var formData bytes.Buffer
	fileWriter := multipart.NewWriter(&formData)

	// Create the form field "file" to match API expectations
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
	bufferSize := int64(formData.Len())
	reader, closer := p.Reader(bufferSize, &formData, "source file")
	defer closer()

	// Build query parameters
	params := &pkgclient.UploadDebianSrcFileParams{
		Distribution: distribution,
		Component:    component,
		Package:      packageName,
		Version:      version,
	}

	resp, err := client.UploadDebianSrcFileWithBodyWithResponse(
		context.Background(),
		config.Global.AccountID,
		registryName,
		params,
		fileWriter.FormDataContentType(),
		reader,
		func(ctx context.Context, req *http.Request) error {
			utils.SetChecksumHeaders(req.Header, checksums)
			return nil
		},
	)

	if err != nil {
		progress.Error("Failed to upload source file")
		return err
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		progress.Error("Upload failed")
		return fmt.Errorf("failed to push source file: %s \n response: %s", resp.Status(), resp.Body)
	}

	progress.Success(fmt.Sprintf("Uploaded %s (package: %s, version: %s)", filePath, packageName, version))
	return nil
}
