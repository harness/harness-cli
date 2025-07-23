// Package gopkg provides functionality for generating and managing Go packages.
package gopkg

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	"github.com/harness/harness-cli/util/common/vcs"
)

// DefaultGenerator provides the default implementation of package generation interfaces.
// It combines the default implementations of ModuleValidator, FileGenerator, and
// VCSMetadataProvider to provide a complete package generation solution.
type DefaultGenerator struct {
	validator   ModuleValidator
	fileGen     FileGenerator
	vcsProvider VCSMetadataProvider
}

// NewDefaultGenerator creates a new DefaultGenerator with default implementations
func NewDefaultGenerator() *DefaultGenerator {
	return &DefaultGenerator{
		validator:   &defaultModuleValidator{},
		fileGen:     &defaultFileGenerator{},
		vcsProvider: &defaultVCSProvider{},
	}
}

// defaultModuleValidator provides the default implementation of ModuleValidator.
// It validates Go module version numbers and module paths according to Go module
// conventions, ensuring that version numbers follow semantic versioning and
// module paths include appropriate version suffixes.
type defaultModuleValidator struct{}

// ValidateVersion checks if the version string follows semantic versioning format.
// The version must be in the form vX.Y.Z where X, Y, and Z are non-negative integers.
// For example: v1.0.0, v2.1.3
// Returns a validation error if the format is invalid.
func (v *defaultModuleValidator) ValidateVersion(version string) error {
	re := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	if !re.MatchString(version) {
		return errors.NewValidationError("version", "must be in form vX.Y.Z")
	}
	return nil
}

// ValidateModulePath validates the module path against the version number.
// For v0 and v1 versions, the module path must not have a /vN suffix.
// For v2+ versions, the module path must have a /vN suffix matching the major version.
// For example:
//   - For v1.0.0: github.com/user/module (no suffix)
//   - For v2.0.0: github.com/user/module/v2
//   - For v3.0.0: github.com/user/module/v3
func (v *defaultModuleValidator) ValidateModulePath(modulePath, version string) error {
	major, err := v.extractMajor(version)
	if err != nil {
		return errors.NewValidationError("version", err.Error())
	}

	if major <= 1 {
		if strings.HasSuffix(modulePath, "/v2") || strings.HasSuffix(modulePath, fmt.Sprintf("/v%d", major+1)) {
			return errors.NewValidationError("module_path", "module path must not have /vN suffix for v1 or below")
		}
		return nil
	}

	expected := fmt.Sprintf("/v%d", major)
	if !strings.HasSuffix(modulePath, expected) {
		return errors.NewValidationError("module_path", "module path must end with major version suffix")
	}
	return nil
}

// extractMajor extracts the major version number from a semantic version string.
// The version string must start with 'v' followed by the major version number.
//
// Parameters:
//   - version: version string in the form vX.Y.Z
//
// Returns:
//   - major version number as an integer
//   - error if the version format is invalid
//
// Example:
//   - "v1.2.3" -> 1
//   - "v2.0.0" -> 2
//   - "invalid" -> error
func (v *defaultModuleValidator) extractMajor(version string) (int, error) {
	re := regexp.MustCompile(`^v(\d+)\.`)
	match := re.FindStringSubmatch(version)
	if len(match) < 2 {
		return 0, errors.NewValidationError("version", "invalid version format")
	}
	var major int
	fmt.Sscanf(match[1], "%d", &major)
	return major, nil
}

// defaultFileGenerator provides the default implementation of FileGenerator.
// It handles the generation of all required files for a Go package, including
// the go.mod file, package info file, and the package zip archive. The generator
// follows Go module proxy conventions for file layout and naming.
type defaultFileGenerator struct{}

// GenerateModFile copies the go.mod file from the source to the output location.
// The file is copied as-is to preserve the original module definition and dependencies.
// The go.mod file is required for the Go module proxy to understand the module's
// dependencies and version requirements.
//
// Parameters:
//   - sourcePath: path to the source go.mod file
//   - outputPath: path where the go.mod file should be written
func (g *defaultFileGenerator) GenerateModFile(sourcePath, outputPath string) error {
	data, err := fileutil.ReadFile(sourcePath)
	if err != nil {
		return errors.NewFileError(sourcePath, "read", err)
	}

	if err := fileutil.WriteFile(outputPath, data); err != nil {
		return errors.NewFileError(outputPath, "write", err)
	}
	return nil
}

// GenerateInfoFile creates a JSON file containing package metadata.
// The info file includes:
//   - Version: the package version
//   - Time: timestamp when the package was created (RFC3339 format)
//   - Origin: VCS metadata if available (Git repository info)
//
// This file is used by the Go module proxy to provide information about
// the package version and its origin.
//
// Parameters:
//   - outputPath: path where the info file should be written
//   - version: version of the package
func (g *defaultFileGenerator) GenerateInfoFile(outputPath, version string, origin *Origin) error {
	info := PackageMetadata{
		Version: version,
		Time:    time.Now().Format(time.RFC3339),
	}
	if origin != nil {
		info.Origin = *origin
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return errors.NewPackageError("marshal_info", version, "", err)
	}

	if err := fileutil.WriteFile(outputPath, data); err != nil {
		return errors.NewFileError(outputPath, "write", err)
	}
	return nil
}

// GenerateZipFile creates a zip archive containing all package files.
// The zip file follows Go module proxy conventions for file layout:
//   - Files are stored under a prefix of "$modulePath@$version/"
//   - All paths use forward slashes, even on Windows
//   - Files maintain their relative paths from the source directory
//
// Parameters:
//   - sourcePath: root directory of the package source
//   - outputPath: path where the zip file should be written
//   - modulePath: import path of the module
//   - version: version of the package
//
// For example, for module "github.com/user/module" at version "v1.0.0":
//   - Source file: src/foo/bar.go
//   - In zip as: github.com/user/module@v1.0.0/foo/bar.go
func (g *defaultFileGenerator) GenerateZipFile(sourcePath, outputPath, modulePath, version string) error {
	fzip, err := os.Create(outputPath)
	if err != nil {
		return errors.NewFileError(outputPath, "create", err)
	}
	defer fzip.Close()

	zw := zip.NewWriter(fzip)
	defer zw.Close()

	entries, err := g.collectZipEntries(sourcePath, modulePath, version)
	if err != nil {
		return errors.NewPackageError("collect_files", version, "", err)
	}

	// Add each file to the zip archive
	for _, entry := range entries {
		if err := g.addFileToZip(zw, entry); err != nil {
			return errors.NewPackageError("add_to_zip", version, "", err)
		}
	}

	return nil
}

// collectZipEntries walks the source directory and collects files to be added to the zip.
// For each file, it creates a zipEntry that maps the source file path to its
// corresponding path in the zip archive. The zip path follows Go module proxy
// conventions by prefixing files with "$modulePath@$version/".
//
// Parameters:
//   - sourcePath: root directory to collect files from
//   - modulePath: import path of the module
//   - version: version of the package
//
// Returns:
//   - list of zipEntry structs mapping source paths to zip paths
//   - error if walking the directory or getting relative paths fails
//
// Example:
//
//	For module "example.com/pkg" at version "v1.0.0":
//	- Source: /path/to/src/foo/bar.go
//	- Zip path: example.com/pkg@v1.0.0/foo/bar.go
func (g *defaultFileGenerator) collectZipEntries(sourcePath, modulePath, version string) ([]zipEntry, error) {
	var entries []zipEntry
	prefix := fmt.Sprintf("%s@%s/", modulePath, version)

	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		rel, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return errors.NewFileError(path, "get_relative_path", err)
		}

		entries = append(entries, zipEntry{
			sourcePath: path,
			zipPath:    prefix + filepath.ToSlash(rel),
		})
		return nil
	})

	if err != nil {
		return nil, errors.NewFileError(sourcePath, "walk", err)
	}

	return entries, nil
}

// addFileToZip adds a single file to the zip archive.
// It opens the source file, creates a new entry in the zip archive with
// the specified path, and copies the file content. The function handles
// proper error handling and resource cleanup.
//
// Parameters:
//   - zw: zip writer to add the file to
//   - entry: zipEntry containing source and destination paths
//
// Returns:
//   - error if opening the source file, creating the zip entry,
//     or copying the content fails
//
// The function ensures that:
//   - Source file is properly opened and closed
//   - Zip entry is created with the correct path
//   - File content is copied efficiently using io.Copy
func (g *defaultFileGenerator) addFileToZip(zw *zip.Writer, entry zipEntry) error {
	src, err := os.Open(entry.sourcePath)
	if err != nil {
		return errors.NewFileError(entry.sourcePath, "open", err)
	}
	defer src.Close()

	w, err := zw.Create(entry.zipPath)
	if err != nil {
		return errors.NewFileError(entry.zipPath, "create_zip_entry", err)
	}

	if _, err := io.Copy(w, src); err != nil {
		return errors.NewFileError(entry.zipPath, "write_zip_entry", err)
	}

	return nil
}

// defaultVCSProvider provides the default implementation of VCSMetadataProvider.
// It extracts version control information from Git repositories, including
// repository URL, current branch or reference, and commit hash. This information
// is used to track the origin of package versions.
type defaultVCSProvider struct{}

// GetMetadata extracts VCS metadata from a Git repository.
// The metadata includes:
//   - VCS: the version control system type (always "git")
//   - URL: the remote repository URL (e.g., "https://github.com/user/repo.git")
//   - Ref: the current branch or tag (e.g., "main", "v1.0.0")
//   - Hash: the full commit hash
//
// This information helps users track where a package version came from and
// verify its authenticity.
//
// Parameters:
//   - path: root directory of the Git repository
func (p *defaultVCSProvider) GetMetadata(path string) (*Origin, error) {
	repo := vcs.NewGitRepository(path)
	gitInfo, err := repo.GetInfo()
	if err != nil {
		return nil, errors.NewPackageError("get_vcs_info", path, "", err)
	}

	return &Origin{
		VCS:  gitInfo.VCS,
		URL:  gitInfo.URL,
		Ref:  gitInfo.Ref,
		Hash: gitInfo.Hash,
	}, nil
}
