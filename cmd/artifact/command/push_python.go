package command

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
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
	p "github.com/harness/harness-cli/util/common/progress"
	"github.com/spf13/cobra"
)

const (
	whlFileExtension         = ".whl"
	targzFileExtension       = ".tar.gz"
	expectedNumberOfArgument = 2
)

func NewPushPythonCmd(c *cmdutils.Factory) *cobra.Command {
	var pkgURL string
	cmd := &cobra.Command{
		Use:   "python <registry_name> <file/folder_path>",
		Short: "Push Python Artifacts",
		Long:  "Push Python Artifacts to Harness Artifact Registry",
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

			// Create progress reporter
			progress := p.NewConsoleReporter()

			// Validate file exists
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return errors.NewValidationError("file_path", fmt.Sprintf("failed to access package file: %v", err))
			}
			var pythonPkgFiles []string
			if fileInfo.IsDir() {
				//scan whole directory and return all .whl and .tar.gz files
				progress.Step(fmt.Sprintf("Scanning folder %s for python packages ", filePath))
				pythonPkgFiles, err = scanFolderForPackages(filePath, progress)
				if err != nil {
					return err
				}

				if len(pythonPkgFiles) == 0 {
					return errors.NewValidationError("Empty Folder", fmt.Sprintf("No python packages found at : %s", filePath))
				}

			} else {
				//handle  single package file scenario
				pythonPkgFiles = append(pythonPkgFiles, filePath)

			}

			progress.Success("Input parameters validated")

			for _, fileNameWithPath := range pythonPkgFiles {
				progress.Step(fmt.Sprintf("Processing %s ", filepath.Base(fileNameWithPath)))
				err := uploadSinglePythonPackageFile(fileNameWithPath, registryName, progress)
				if err != nil {
					return err
				}
			}
			progress.Success(fmt.Sprintf("Successfully uploaded package %s", filePath))
			return nil

		},
	}

	cmd.SilenceErrors = true //prevent printing errors twice
	cmd.Flags().StringVar(&pkgURL, "pkg-url", "", "Base URL for the Packages")
	cmd.MarkFlagRequired("pkg-url")
	return cmd
}

func uploadSinglePythonPackageFile(fileNameWithPath string, registryName string, progress *p.ConsoleReporter) error {
	// Initialize the package client
	pkgClient, err := pkgclient.NewClientWithResponses(config.Global.Registry.PkgURL,
		auth.GetAuthOptionARPKG())
	if err != nil {
		return fmt.Errorf("failed to create package client: %w", err)
	}

	file, err := os.Open(fileNameWithPath)
	if err != nil {
		progress.Error("Failed to open package file")
		return err
	}
	defer file.Close()

	version := ""
	name := ""
	metadata, err := extractPythonPackageMetadata(fileNameWithPath)

	if err != nil {
		return fmt.Errorf("failed to get metadata from package : %w", err)
	}
	version = metadata.Version
	name = metadata.Name

	var formData bytes.Buffer
	fileWriter := multipart.NewWriter(&formData)

	err = fileWriter.WriteField("name", name)
	if err != nil {
		return err
	}
	err = fileWriter.WriteField("version", version)
	if err != nil {
		return err
	}

	// Create the form field "content" to match curl
	part, err := fileWriter.CreateFormFile("content", filepath.Base(fileNameWithPath))
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
	progress.Step(fmt.Sprintf("Uploading %s ", filepath.Base(fileNameWithPath)))
	bufferSize := int64(formData.Len())

	reader, closer := p.Reader(bufferSize, &formData, filepath.Ext(fileNameWithPath))
	defer closer()

	resp, err := pkgClient.UploadPythonPackageWithBodyWithResponse(
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

	return nil
}

func extractPythonPackageMetadata(path string) (*pythonPackageMetadata, error) {
	switch {
	case strings.HasSuffix(path, targzFileExtension):
		return extractMetadataFromTarGz(path)
	case strings.HasSuffix(path, whlFileExtension):
		return extractMetadataFromWhl(path)
	default:
		return nil, fmt.Errorf("unsupported python package format")
	}
}

func extractMetadataFromTarGz(path string) (*pythonPackageMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if filepath.Base(hdr.Name) == "PKG-INFO" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}

			return parseMetadata(data)
		}
	}

	return nil, fmt.Errorf("PKG-INFO not found")
}

func extractMetadataFromWhl(path string) (*pythonPackageMetadata, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open whl file: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// METADATA always lives under *.dist-info/
		if filepath.Base(f.Name) == "METADATA" &&
			strings.Contains(f.Name, ".dist-info/") {

			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open METADATA: %w", err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read METADATA: %w", err)
			}

			return parseMetadata(data)
		}
	}

	return nil, fmt.Errorf("METADATA not found in whl file")
}

func parseMetadata(data []byte) (*pythonPackageMetadata, error) {
	meta := &pythonPackageMetadata{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Name: ") {
			meta.Name = strings.TrimPrefix(line, "Name: ")
		}

		if strings.HasPrefix(line, "Version: ") {
			meta.Version = strings.TrimPrefix(line, "Version: ")
		}

		if meta.Name != "" && meta.Version != "" {
			return meta, nil
		}
	}

	return nil, fmt.Errorf("required metadata not found in package file")
}

type pythonPackageMetadata struct {
	Name    string
	Version string
}

func isValidPythonPackageFile(fileName string) (bool, error) {
	if fileName == "" {
		return false, fmt.Errorf("empty filename")
	}

	name := fileName
	if strings.HasSuffix(name, whlFileExtension) || strings.HasSuffix(name, targzFileExtension) {
		return true, nil
	}
	//in case of file is having other  extension than provided extension
	return false, fmt.Errorf("unsupported extension: %s", filepath.Ext(name))
}

// scanFolderForPackages scans a folder and returns all .tar.gz and .whl files
func scanFolderForPackages(folderPath string, progress *p.ConsoleReporter) ([]string, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, err
	}

	var files []string

	for _, entry := range entries {
		if entry.IsDir() {

			progress.Step(fmt.Sprintf("Skipping sub directory  %s ", entry.Name()))
			continue
		}

		name := entry.Name()

		ok, _ := isValidPythonPackageFile(name)
		if ok {
			fullPath := filepath.Join(folderPath, name)
			files = append(files, fullPath)
		} else {
			if strings.HasPrefix(name, ".") {
				continue // ignore hidden files like .DS_Store
			}
			progress.Step(fmt.Sprintf("Skipping file with unsupported extension  %s ", filepath.Base(name)))
		}
	}

	return files, nil
}
