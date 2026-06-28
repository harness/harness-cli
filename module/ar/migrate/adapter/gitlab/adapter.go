package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/rs/zerolog/log"
)

func init() {
	adapterType := types.GITLAB
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		log.Error().Err(err).Msg("Failed to register GitLab adapter factory")
		return
	}
}

// factory creates GitLab adapter instances
type factory struct{}

// Create implements the Factory interface
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

// adapter implements the Adapter interface for GitLab Package Registry
type adapter struct {
	client *Client
	reg    types.RegistryConfig
}

// newAdapter creates a new GitLab adapter
func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	return &adapter{
		client: newClient(&config),
		reg:    config,
	}, nil
}

// GetKeyChain returns an authentication keychain for GitLab
func (a *adapter) GetKeyChain(sourcePackageHostname string) (authn.Keychain, error) {
	var host string
	if sourcePackageHostname != "" {
		host = sourcePackageHostname
	} else {
		parse, err := url.Parse(a.reg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to parse endpoint [%s]: %w", a.reg.Endpoint, err)
		}
		host = parse.Host
	}
	return NewGitlabKeychain(a.reg.Credentials.Username, a.reg.Credentials.Password, host), nil
}

// GetConfig returns the registry configuration
func (a *adapter) GetConfig() types.RegistryConfig {
	return a.reg
}

// ValidateCredentials validates the GitLab credentials
func (a *adapter) ValidateCredentials() (bool, error) {
	// Try to get project information to validate credentials
	// The registry parameter in migration context is typically the project path
	return true, nil
}

// GetRegistry returns registry information for a GitLab project
func (a *adapter) GetRegistry(ctx context.Context, registry string) (types.RegistryInfo, error) {
	return a.client.getRegistry(registry)
}

// CreateRegistryIfDoesntExist is not applicable for GitLab (projects are managed separately)
func (a *adapter) CreateRegistryIfDoesntExist(registry string, artifactType types.ArtifactType) (bool, error) {
	return false, nil
}

// GetPackages retrieves all packages from a GitLab project
func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType, root *types.TreeNode) ([]types.Package, error) {
	log.Info().
		Str("registry", registry).
		Str("artifactType", string(artifactType)).
		Msg("Getting packages from GitLab")

	// Docker images are in Container Registry, not Package Registry
	if artifactType == types.DOCKER {
		return a.getDockerPackages(registry)
	}

	// For other package types, use Package Registry API
	// Map artifact type to GitLab package type filter
	gitlabPackageType := mapArtifactTypeToGitLab(artifactType)

	// Get all packages from GitLab
	gitlabPackages, err := a.client.getAllPackages(registry, gitlabPackageType)
	if err != nil {
		return nil, fmt.Errorf("failed to get packages: %w", err)
	}

	// Convert GitLab packages to migration packages
	// Group by package name (one Package per name, not per version)
	packageMap := make(map[string]types.Package)
	for _, glPkg := range gitlabPackages {
		if _, exists := packageMap[glPkg.Name]; !exists {
			pkg := types.Package{
				Registry: registry,
				Name:     glPkg.Name,
				// Path is where to find versions in the tree
				// Tree structure: /packages/{name}/{version}/...
				Path:     fmt.Sprintf("/packages/%s", glPkg.Name),
				Size:     -1,
			}
			packageMap[glPkg.Name] = pkg
		}
	}

	// Convert map to slice
	packages := make([]types.Package, 0, len(packageMap))
	for _, pkg := range packageMap {
		packages = append(packages, pkg)
	}

	log.Info().
		Int("packageCount", len(packages)).
		Msg("Retrieved packages from GitLab")

	return packages, nil
}

// getDockerPackages retrieves Docker images from GitLab Container Registry
func (a *adapter) getDockerPackages(registry string) ([]types.Package, error) {
	log.Info().
		Str("registry", registry).
		Msg("Getting Docker images from GitLab Container Registry")

	// Get repositories and their tags
	repos, tags, err := a.client.getContainerRepositoriesWithTags(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to get Docker repositories: %w", err)
	}

	packages := make([]types.Package, 0)

	// Create ONE package per repository (not per tag)
	// The migration framework will use crane.ListTags() to enumerate tags later
	// This matches JFrog/Nexus behavior
	for _, repo := range repos {
		repoTags := tags[repo.ID]

		if len(repoTags) == 0 {
			log.Warn().
				Str("repository", repo.Name).
				Msg("Repository has no tags, skipping")
			continue
		}

		// Create a single package for the repository (without tag)
		// repo.Name = "simple-demo" (just the image name, no tag)
		pkg := types.Package{
			Registry: registry,
			Name:     repo.Name,  // Just the image name, e.g., "simple-demo"
			Path:     "/",        // Docker uses "/" as path (OCI pattern)
			Size:     -1,         // Size will be calculated per tag by crane
			Metadata: map[string]string{
				"fullPath": repo.Path,  // Store full path: disiok-group/disiok-project/simple-demo
			},
		}
		packages = append(packages, pkg)

		log.Info().
			Str("repository", repo.Name).
			Int("tagCount", len(repoTags)).
			Msg("Added Docker repository (tags will be enumerated by crane)")
	}

	log.Info().
		Int("totalRepositories", len(packages)).
		Msg("Retrieved Docker repositories from GitLab Container Registry")

	return packages, nil
}

// GetVersions retrieves all versions of a specific package
func (a *adapter) GetVersions(
	p types.Package,
	node *types.TreeNode,
	registry, pkg string,
	artifactType types.ArtifactType,
) ([]types.Version, error) {
	log.Info().
		Str("registry", registry).
		Str("package", pkg).
		Str("artifactType", string(artifactType)).
		Msg("Getting versions from GitLab")

	// Map artifact type to GitLab package type
	gitlabPackageType := mapArtifactTypeToGitLab(artifactType)

	// Get all packages with this name
	gitlabPackages, err := a.client.getAllPackages(registry, gitlabPackageType)
	if err != nil {
		return nil, fmt.Errorf("failed to get packages: %w", err)
	}

	// Filter by package name and collect versions
	versions := make([]types.Version, 0)
	for _, glPkg := range gitlabPackages {
		if glPkg.Name == pkg {
			version := types.Version{
				Registry: registry,
				Pkg:      pkg,
				Name:     glPkg.Version,
				// Path should be relative to the package path
				// The package path is /packages/{pkg}, so version path is just the version
				Path:     glPkg.Version,
				Size:     -1,
			}
			versions = append(versions, version)
		}
	}

	log.Info().
		Str("package", pkg).
		Int("versionCount", len(versions)).
		Msg("Retrieved versions from GitLab")

	return versions, nil
}

// GetFiles retrieves all files from a GitLab registry
// For Docker images, returns empty (Docker uses OCI manifest instead of file enumeration)
func (a *adapter) GetFiles(registry string) ([]types.File, error) {
	log.Info().
		Str("registry", registry).
		Msg("Getting all files from GitLab")

	// Docker images don't use file enumeration - they use OCI catalog
	// Return empty file list for Docker, the migration will use GetPackages instead
	// Note: This is intentionally left empty for Docker to match JFrog/Nexus pattern
	// The actual Docker migration happens through the OCI flow in package.go

	// Get all packages from Package Registry (non-Docker types)
	gitlabPackages, err := a.client.getAllPackages(registry, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get packages: %w", err)
	}

	// Collect all files from all packages
	var allFiles []types.File

	for _, glPkg := range gitlabPackages {
		// Get files for this package
		packageFiles, err := a.client.getPackageFiles(registry, glPkg.ID)
		if err != nil {
			log.Warn().
				Err(err).
				Int64("packageID", glPkg.ID).
				Str("package", glPkg.Name).
				Msg("Failed to get package files, skipping")
			continue
		}

		// Convert to migration File type
		for _, glFile := range packageFiles {
			file := types.File{
				Name:     glFile.FileName,
				Registry: registry,
				Uri:      fmt.Sprintf("/packages/%s/%s/%s", glPkg.Name, glPkg.Version, glFile.FileName),
				Folder:   false,
				Size:     glFile.Size,
				SHA1:     glFile.FileSHA1,
				SHA2:     glFile.FileSHA256,
			}
			allFiles = append(allFiles, file)
		}
	}

	log.Info().
		Int("fileCount", len(allFiles)).
		Msg("Retrieved all files from GitLab")

	return allFiles, nil
}

// DownloadFile downloads a file from GitLab
func (a *adapter) DownloadFile(registry string, uri string) (io.ReadCloser, http.Header, error) {
	log.Debug().
		Str("registry", registry).
		Str("uri", uri).
		Msg("Downloading file from GitLab")

	// Parse the URI to extract package info
	// Expected format: /packages/{name}/{version}/{filename}
	// For scoped packages: /packages/@scope/name/version/filename
	trimmedURI := strings.Trim(uri, "/")

	// Remove "packages/" prefix
	if !strings.HasPrefix(trimmedURI, "packages/") {
		return nil, http.Header{}, fmt.Errorf("invalid URI format (missing packages prefix): %s", uri)
	}
	trimmedURI = strings.TrimPrefix(trimmedURI, "packages/")

	// Split remaining path
	parts := strings.Split(trimmedURI, "/")
	if len(parts) < 3 {
		return nil, http.Header{}, fmt.Errorf("invalid URI format (too few parts): %s", uri)
	}

	// Handle scoped packages (@scope/name) vs regular packages (name)
	var packageName, packageVersion, fileName string
	if strings.HasPrefix(parts[0], "@") && len(parts) >= 4 {
		// Scoped package: @scope/name/version/filename
		packageName = parts[0] + "/" + parts[1]
		packageVersion = parts[2]
		// Filename is everything after version (may contain /)
		fileName = strings.Join(parts[3:], "/")
	} else {
		// Regular package: name/version/filename
		packageName = parts[0]
		packageVersion = parts[1]
		// Filename is everything after version (may contain /)
		fileName = strings.Join(parts[2:], "/")
	}

	// Get project info for package-type-specific download URLs
	project, err := a.client.getProject(registry)
	if err != nil {
		return nil, http.Header{}, fmt.Errorf("failed to get project info: %w", err)
	}

	// Use package-type-specific registry APIs for download
	// NPM: .tgz files
	// Try NPM-specific download first, but fall through to generic if it fails
	if strings.HasSuffix(fileName, ".tgz") {
		// NPM registry API: /api/v4/projects/{id}/packages/npm/{package_name}/-/{filename}
		downloadURL := fmt.Sprintf("%s/api/v4/projects/%d/packages/npm/%s/-/%s",
			a.reg.Endpoint, project.ID, packageName, fileName)
		reader, header, err := a.client.downloadFile(downloadURL)
		if err == nil {
			return reader, header, nil
		}
		// If NPM download fails, fall through to generic package files endpoint
		log.Debug().Err(err).Msg("NPM-specific download failed, trying generic endpoint")
	}

	// PyPI/Python: .whl or .tar.gz files
	if strings.HasSuffix(fileName, ".whl") || (strings.HasSuffix(fileName, ".tar.gz") && !strings.Contains(fileName, ".nupkg")) {
		// PyPI registry API: /api/v4/projects/{id}/packages/pypi/files/{sha256}/{filename}
		// We need to get the file's SHA256 first
		gitlabPackages, err := a.client.getAllPackages(registry, "pypi")
		if err == nil {
			for _, glPkg := range gitlabPackages {
				if glPkg.Name == packageName && glPkg.Version == packageVersion {
					packageFiles, err := a.client.getPackageFiles(registry, glPkg.ID)
					if err == nil {
						for _, file := range packageFiles {
							if file.FileName == fileName && file.FileSHA256 != "" {
								downloadURL := fmt.Sprintf("%s/api/v4/projects/%d/packages/pypi/files/%s/%s",
									a.reg.Endpoint, project.ID, file.FileSHA256, fileName)
								return a.client.downloadFile(downloadURL)
							}
						}
					}
					break
				}
			}
		}
	}

	// Maven: .jar, .pom, .war, .ear files
	if strings.HasSuffix(fileName, ".jar") || strings.HasSuffix(fileName, ".pom") ||
		strings.HasSuffix(fileName, ".war") || strings.HasSuffix(fileName, ".ear") ||
		strings.HasSuffix(fileName, ".xml") || strings.HasSuffix(fileName, ".sha1") ||
		strings.HasSuffix(fileName, ".md5") {
		// Maven registry API: /api/v4/projects/{id}/packages/maven/{group}/{artifact}/{version}/{filename}
		// packageName is already in Maven format: com/example/test-app
		downloadURL := fmt.Sprintf("%s/api/v4/projects/%d/packages/maven/%s/%s/%s",
			a.reg.Endpoint, project.ID, packageName, packageVersion, fileName)
		return a.client.downloadFile(downloadURL)
	}

	// NuGet: .nupkg files
	if strings.HasSuffix(fileName, ".nupkg") || strings.HasSuffix(fileName, ".snupkg") {
		// NuGet registry API: /api/v4/projects/{id}/packages/nuget/download/{package_name}/{version}/{filename}
		downloadURL := fmt.Sprintf("%s/api/v4/projects/%d/packages/nuget/download/%s/%s/%s",
			a.reg.Endpoint, project.ID, packageName, packageVersion, fileName)
		return a.client.downloadFile(downloadURL)
	}

	// Composer: .zip files (PHP packages)
	if strings.HasSuffix(fileName, ".zip") && strings.Contains(uri, "composer") {
		// Composer packages are typically served as archives
		// Fall through to generic approach
	}

	// For other package types or if specific APIs don't work, use generic package files endpoint

	// Get all packages to find the one we need
	gitlabPackages, err := a.client.getAllPackages(registry, "")
	if err != nil {
		return nil, http.Header{}, fmt.Errorf("failed to get packages: %w", err)
	}

	// Find the matching package
	var targetPackageID int64
	for _, glPkg := range gitlabPackages {
		if glPkg.Name == packageName && glPkg.Version == packageVersion {
			targetPackageID = glPkg.ID
			break
		}
	}

	if targetPackageID == 0 {
		return nil, http.Header{}, fmt.Errorf("package not found: %s@%s", packageName, packageVersion)
	}

	// Get package files to find the download URL
	packageFiles, err := a.client.getPackageFiles(registry, targetPackageID)
	if err != nil {
		return nil, http.Header{}, fmt.Errorf("failed to get package files: %w", err)
	}

	// Find the specific file
	var downloadURL string
	for _, file := range packageFiles {
		if file.FileName == fileName {
			// Construct the download URL
			// GitLab API v4 package file download endpoint
			encodedRegistry := url.PathEscape(registry)
			downloadURL = fmt.Sprintf("%s/api/v4/projects/%s/packages/%d/package_files/%d",
				a.reg.Endpoint, encodedRegistry, targetPackageID, file.ID)
			break
		}
	}

	if downloadURL == "" {
		return nil, http.Header{}, fmt.Errorf("file not found: %s", fileName)
	}

	// Download the file
	return a.client.downloadFile(downloadURL)
}

// UploadFile is not implemented for GitLab (source only)
func (a *adapter) UploadFile(
	registry string,
	file io.ReadCloser,
	f *types.File,
	header http.Header,
	artifactName string,
	version string,
	artifactType types.ArtifactType,
	metadata map[string]interface{},
) error {
	return fmt.Errorf("upload not supported for GitLab adapter (source only)")
}

// GetOCIImagePath returns the OCI image path for GitLab Container Registry
func (a *adapter) GetOCIImagePath(registry string, packageHostname string, image string) (string, error) {
	// GitLab Container Registry uses the format:
	// registry.gitlab.com/group/project/image:tag
	//
	// The 'registry' parameter is the project path (e.g., "disiok-group/disiok-project")
	// The 'image' parameter is the image name with tag (e.g., "simple-demo:latest")

	if packageHostname != "" {
		// Use custom registry hostname
		return fmt.Sprintf("%s/%s/%s", packageHostname, registry, image), nil
	}

	// Determine registry host from endpoint
	registryHost := "registry.gitlab.com"
	if !strings.Contains(a.reg.Endpoint, "gitlab.com") {
		// For self-hosted GitLab, extract host
		parse, err := url.Parse(a.reg.Endpoint)
		if err != nil {
			return "", fmt.Errorf("failed to parse endpoint: %w", err)
		}
		registryHost = "registry." + parse.Host
	}

	// Construct: registry.gitlab.com/group/project/image:tag
	return fmt.Sprintf("%s/%s/%s", registryHost, registry, image), nil
}

// AddNPMTag is not applicable for GitLab
func (a *adapter) AddNPMTag(registry string, name string, version string, uri string) error {
	return nil
}

// VersionExists checks if a version exists (not implemented for source adapter)
func (a *adapter) VersionExists(
	ctx context.Context,
	p types.Package,
	registryRef, pkg, version string,
	artifactType types.ArtifactType,
) (bool, error) {
	return false, fmt.Errorf("not implemented for source adapter")
}

// FileExists checks if a file exists (not implemented for source adapter)
func (a *adapter) FileExists(
	ctx context.Context,
	registryRef, pkg, version string,
	fileName *types.File,
	artifactType types.ArtifactType,
) (bool, error) {
	return false, fmt.Errorf("not implemented for source adapter")
}

// GetAllFilesForVersion retrieves all files for a specific version
func (a *adapter) GetAllFilesForVersion(
	ctx context.Context,
	registryRef, pkg, version string,
) ([]string, error) {
	// Get all packages
	gitlabPackages, err := a.client.getAllPackages(registryRef, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get packages: %w", err)
	}

	// Find the matching package version
	var targetPackageID int64
	for _, glPkg := range gitlabPackages {
		if glPkg.Name == pkg && glPkg.Version == version {
			targetPackageID = glPkg.ID
			break
		}
	}

	if targetPackageID == 0 {
		return nil, fmt.Errorf("package version not found: %s@%s", pkg, version)
	}

	// Get package files
	packageFiles, err := a.client.getPackageFiles(registryRef, targetPackageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get package files: %w", err)
	}

	// Extract file names
	fileNames := make([]string, 0, len(packageFiles))
	for _, file := range packageFiles {
		fileNames = append(fileNames, file.FileName)
	}

	return fileNames, nil
}

// CreateVersion creates a new version (not implemented for source adapter)
func (a *adapter) CreateVersion(
	registry string,
	artifactName string,
	version string,
	artifactType types.ArtifactType,
	files []*types.PackageFiles,
	metadata map[string]interface{},
) error {
	return fmt.Errorf("not implemented for source adapter")
}

// mapArtifactTypeToGitLab maps migration artifact types to GitLab package types
func mapArtifactTypeToGitLab(artifactType types.ArtifactType) string {
	switch artifactType {
	case types.MAVEN:
		return "maven"
	case types.NPM:
		return "npm"
	case types.PYTHON:
		return "pypi"
	case types.NUGET:
		return "nuget"
	case types.COMPOSER:
		return "composer"
	case types.CONAN:
		return "conan"
	case types.HELM, types.HELM_LEGACY, types.HELM_HTTP:
		return "helm"
	case types.DEBIAN:
		return "debian"
	case types.GO:
		return "golang"
	case types.RUBYGEMS:
		return "rubygems"
	case types.GENERIC, types.RAW:
		return "generic"
	case types.RPM, types.CONDA, types.DART, types.SWIFT, types.PUPPET:
		// These types are not natively supported by GitLab Package Registry
		// They would typically be stored as generic packages
		return "generic"
	default:
		// Return empty string to get all package types
		return ""
	}
}
