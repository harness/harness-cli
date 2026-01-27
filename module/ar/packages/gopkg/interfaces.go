package gopkg

// PackageGenerator defines the interface for Go package generation.
// Implementations of this interface should handle the complete process
// of generating a Go package, including validation, file generation,
// and metadata collection.
type PackageGenerator interface {
	// Generate creates the package files and returns the package name.
	// It performs all necessary validation and file generation steps,
	// creating the .mod, .info, and .zip files in the output directory.
	// Returns the package name (version) on success, or an error if any
	// step in the generation process fails.
	Generate() (string, error)
}

// ModuleValidator defines the interface for Go module validation.
// This interface provides methods to validate Go module version numbers
// and module paths according to Go module conventions.
type ModuleValidator interface {
	// ValidateVersion validates the package version format.
	// The version must be in the form vX.Y.Z where X, Y, and Z are non-negative integers.
	// Returns an error if the version format is invalid.
	ValidateVersion(version string) error

	// ValidateModulePath validates the module path against version.
	// For v0 and v1 versions, the module path must not have a /vN suffix.
	// For v2+ versions, the module path must have a /vN suffix matching the major version.
	// Returns an error if the module path does not follow these conventions.
	ValidateModulePath(modulePath, version string) error
}

// FileGenerator defines the interface for package file generation.
// This interface provides methods to generate all required files for
// a Go package, including the go.mod file, package info file, and
// the package zip archive.
type FileGenerator interface {
	// GenerateModFile generates the go.mod file by copying it from the source
	// to the output location. The file is copied as-is to preserve the original
	// module definition and dependencies.
	// sourcePath: path to the source go.mod file
	// outputPath: path where the go.mod file should be written
	GenerateModFile(sourcePath, outputPath string) error

	// GenerateInfoFile generates the package info file containing metadata
	// about the package, including version information and VCS details.
	// outputPath: path where the info file should be written
	// version: version of the package
	GenerateInfoFile(outputPath, version string, origin *Origin) error

	// GenerateZipFile generates the package zip file containing all package files.
	// The zip file follows Go module proxy conventions for file layout.
	// sourcePath: root directory of the package source
	// outputPath: path where the zip file should be written
	// modulePath: import path of the module
	// version: version of the package
	GenerateZipFile(sourcePath, outputPath, modulePath, version string) error
}

// VCSMetadataProvider defines the interface for version control system metadata.
// This interface provides methods to extract version control information
// from a package's repository, such as Git commit hashes, branch names,
// and remote URLs.
type VCSMetadataProvider interface {
	// GetMetadata returns VCS metadata for the package.
	// It extracts information such as the VCS type (e.g., git),
	// repository URL, current branch or reference, and commit hash.
	// path: root directory of the package repository
	// Returns the VCS metadata or an error if extraction fails.
	GetMetadata(path string) (*Origin, error)
}
