// Package gopkg provides functionality for generating and managing Go packages.
package gopkg

import (
	"fmt"
	"path/filepath"

	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"
	"github.com/harness/harness-cli/util/common/progress"

	"golang.org/x/mod/modfile"
)

// Generator handles Go package generation operations.
// It coordinates the validation, file generation, and metadata collection
// processes using the provided interfaces.
type Generator struct {
	sourceDir string
	outputDir string
	version   string

	validator   ModuleValidator
	fileGen     FileGenerator
	vcsProvider VCSMetadataProvider
}

// NewGenerator creates a new Generator instance with default implementations.
// It uses the DefaultGenerator to provide the standard implementations of
// ModuleValidator, FileGenerator, and VCSMetadataProvider.
func NewGenerator(sourceDir, outputDir, version string) *Generator {
	defGen := NewDefaultGenerator()
	return &Generator{
		sourceDir:   sourceDir,
		outputDir:   outputDir,
		version:     version,
		validator:   defGen.validator,
		fileGen:     defGen.fileGen,
		vcsProvider: defGen.vcsProvider,
	}
}

// Generate creates the package files and returns the package name.
// It performs the following steps:
// 1. Validates the version format and module path
// 2. Prepares the output directory
// 3. Generates all required package files
// Returns the package name (version) on success, or an error if any step fails.
func (g *Generator) Generate(reporter progress.Reporter) (string, error) {
	if reporter == nil {
		reporter = &progress.NopReporter{}
	}

	// Start package generation
	reporter.Start("Generating Go package")
	defer reporter.End()

	// Validate version and module path
	reporter.Step("Validating package version")
	if err := g.validator.ValidateVersion(g.version); err != nil {
		reporter.Error("Invalid version format")
		return "", errors.NewPackageError("validate_version", g.version, "", err)
	}

	goModPath := filepath.Join(g.sourceDir, "go.mod")
	reporter.Step("Extracting module path")
	modulePath, err := g.extractModulePath(goModPath)
	if err != nil {
		reporter.Error("Failed to extract module path")
		return "", errors.NewPackageError("extract_module", g.version, "", err)
	}

	reporter.Step("Validating module path")
	if err := g.validator.ValidateModulePath(modulePath, g.version); err != nil {
		reporter.Error("Invalid module path")
		return "", errors.NewPackageError("validate_module", modulePath, g.version, err)
	}

	// Prepare output directory
	reporter.Step("Preparing output directory")
	if err := fileutil.ResetDir(g.outputDir); err != nil {
		reporter.Error("Failed to prepare output directory")
		return "", errors.NewPackageError("prepare_output", g.version, "", err)
	}

	// Generate package files
	reporter.Step("Generating package files")
	if err := g.generatePackageFiles(modulePath, reporter); err != nil {
		reporter.Error("Failed to generate package files")
		return "", err
	}

	reporter.Success("Package generated successfully")
	return g.version, nil
}

// validatePackage performs all validation checks before package generation

// prepareOutputDirectory prepares the output directory for package generation

// generatePackageFiles generates all required package files
// generatePackageFiles coordinates the generation of all required package files.
// It generates three files:
// 1. go.mod file - copied from the source directory
// 2. .info file - contains package metadata
// 3. .zip file - contains all package files
func (g *Generator) generatePackageFiles(modulePath string, progress progress.Reporter) error {
	// Generate go.mod file
	progress.Step("Generating go.mod file")
	goModPath := filepath.Join(g.sourceDir, "go.mod")
	modOutPath := filepath.Join(g.outputDir, g.version+".mod")
	if err := g.fileGen.GenerateModFile(goModPath, modOutPath); err != nil {
		progress.Error("Failed to generate go.mod file")
		return errors.NewPackageError("write_mod", g.version, "", err)
	}

	// Generate .info file
	progress.Step("Generating package info file")
	infoPath := filepath.Join(g.outputDir, g.version+".info")
	originMetadata, err := g.vcsProvider.GetMetadata(g.sourceDir)
	if err != nil {
		progress.Error(fmt.Sprintf("Failed to get VCS metadata: %s", err.Error()))
		originMetadata = nil
	}
	if err := g.fileGen.GenerateInfoFile(infoPath, g.version, originMetadata); err != nil {
		progress.Error("Failed to generate info file")
		return errors.NewPackageError("write_info", g.version, "", err)
	}

	// Generate zip file
	progress.Step("Creating package archive")
	zipPath := filepath.Join(g.outputDir, g.version+".zip")
	if err := g.fileGen.GenerateZipFile(g.sourceDir, zipPath, modulePath, g.version); err != nil {
		progress.Error("Failed to create package archive")
		return errors.NewPackageError("write_zip", g.version, "", err)
	}

	return nil
}

// extractModulePath reads and parses the go.mod file to extract the module path.
// It validates that the file exists, is properly formatted, and contains
// a module directive.
func (g *Generator) extractModulePath(goModPath string) (string, error) {
	data, err := fileutil.ReadFile(goModPath)
	if err != nil {
		return "", errors.NewFileError(goModPath, "read", err)
	}

	modFile, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", errors.NewValidationError("go.mod", "invalid module file format")
	}

	if modFile.Module == nil {
		return "", errors.NewValidationError("go.mod", "module directive not found")
	}

	return modFile.Module.Mod.Path, nil
}
