package gitlab

import (
	"fmt"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/rs/zerolog/log"
)

// PackageHandler provides specialized handling for different package types
type PackageHandler interface {
	// GetPackageMetadata extracts additional metadata specific to this package type
	GetPackageMetadata(pkg *GitLabPackage) map[string]string

	// NormalizePackageName normalizes the package name according to type-specific rules
	NormalizePackageName(name string) string

	// GetDownloadPath constructs the correct download path for this package type
	GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string
}

// MavenHandler handles Maven-specific package operations
type MavenHandler struct{}

func (h *MavenHandler) GetPackageMetadata(pkg *GitLabPackage) map[string]string {
	metadata := make(map[string]string)

	// Maven packages often have groupId:artifactId format
	if strings.Contains(pkg.Name, ":") {
		parts := strings.SplitN(pkg.Name, ":", 2)
		metadata["groupId"] = parts[0]
		metadata["artifactId"] = parts[1]
	} else {
		metadata["artifactId"] = pkg.Name
	}

	metadata["version"] = pkg.Version
	return metadata
}

func (h *MavenHandler) NormalizePackageName(name string) string {
	// Maven uses groupId:artifactId format
	return name
}

func (h *MavenHandler) GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string {
	// GitLab Maven packages: /api/v4/projects/:id/packages/maven/*path
	return fmt.Sprintf("/api/v4/projects/%s/packages/maven/%s/%s/%s",
		projectPath, strings.ReplaceAll(pkg.Name, ":", "/"), pkg.Version, file.FileName)
}

// NpmHandler handles NPM-specific package operations
type NpmHandler struct{}

func (h *NpmHandler) GetPackageMetadata(pkg *GitLabPackage) map[string]string {
	metadata := make(map[string]string)

	// NPM packages may have scoped names (@scope/package)
	if strings.HasPrefix(pkg.Name, "@") {
		parts := strings.SplitN(pkg.Name, "/", 2)
		if len(parts) == 2 {
			metadata["scope"] = parts[0]
			metadata["packageName"] = parts[1]
		}
	}

	metadata["version"] = pkg.Version
	return metadata
}

func (h *NpmHandler) NormalizePackageName(name string) string {
	// NPM package names should be lowercase
	return strings.ToLower(name)
}

func (h *NpmHandler) GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string {
	// GitLab NPM packages: /api/v4/projects/:id/packages/npm/*path
	return fmt.Sprintf("/api/v4/projects/%s/packages/npm/%s/-/%s/%s",
		projectPath, pkg.Name, pkg.Version, file.FileName)
}

// PyPIHandler handles Python package operations
type PyPIHandler struct{}

func (h *PyPIHandler) GetPackageMetadata(pkg *GitLabPackage) map[string]string {
	metadata := make(map[string]string)
	metadata["name"] = pkg.Name
	metadata["version"] = pkg.Version
	return metadata
}

func (h *PyPIHandler) NormalizePackageName(name string) string {
	// PyPI package names are case-insensitive and normalize underscores/dashes
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "_", "-"), " ", "-"))
}

func (h *PyPIHandler) GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string {
	// GitLab PyPI packages: /api/v4/projects/:id/packages/pypi/files/*path
	return fmt.Sprintf("/api/v4/projects/%s/packages/pypi/files/%s/%s",
		projectPath, pkg.Name, file.FileName)
}

// NuGetHandler handles NuGet-specific package operations
type NuGetHandler struct{}

func (h *NuGetHandler) GetPackageMetadata(pkg *GitLabPackage) map[string]string {
	metadata := make(map[string]string)
	metadata["id"] = pkg.Name
	metadata["version"] = pkg.Version
	return metadata
}

func (h *NuGetHandler) NormalizePackageName(name string) string {
	// NuGet package IDs are case-insensitive
	return strings.ToLower(name)
}

func (h *NuGetHandler) GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string {
	// GitLab NuGet packages: /api/v4/projects/:id/packages/nuget/download/*path
	return fmt.Sprintf("/api/v4/projects/%s/packages/nuget/download/%s/%s/%s",
		projectPath, pkg.Name, pkg.Version, file.FileName)
}

// GenericHandler handles generic package operations
type GenericHandler struct{}

func (h *GenericHandler) GetPackageMetadata(pkg *GitLabPackage) map[string]string {
	metadata := make(map[string]string)
	metadata["name"] = pkg.Name
	metadata["version"] = pkg.Version
	return metadata
}

func (h *GenericHandler) NormalizePackageName(name string) string {
	return name
}

func (h *GenericHandler) GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string {
	// GitLab Generic packages: /api/v4/projects/:id/packages/generic/*path
	return fmt.Sprintf("/api/v4/projects/%s/packages/generic/%s/%s/%s",
		projectPath, pkg.Name, pkg.Version, file.FileName)
}

// ComposerHandler handles Composer-specific package operations
type ComposerHandler struct{}

func (h *ComposerHandler) GetPackageMetadata(pkg *GitLabPackage) map[string]string {
	metadata := make(map[string]string)

	// Composer packages often have vendor/package format
	if strings.Contains(pkg.Name, "/") {
		parts := strings.SplitN(pkg.Name, "/", 2)
		metadata["vendor"] = parts[0]
		metadata["package"] = parts[1]
	}

	metadata["version"] = pkg.Version
	return metadata
}

func (h *ComposerHandler) NormalizePackageName(name string) string {
	// Composer package names are lowercase
	return strings.ToLower(name)
}

func (h *ComposerHandler) GetDownloadPath(projectPath string, pkg *GitLabPackage, file *GitLabPackageFile) string {
	// GitLab Composer packages: /api/v4/projects/:id/packages/composer/*path
	return fmt.Sprintf("/api/v4/projects/%s/packages/composer/%s/%s",
		projectPath, pkg.Name, file.FileName)
}

// GetPackageHandler returns the appropriate handler for a given artifact type
func GetPackageHandler(artifactType types.ArtifactType) PackageHandler {
	switch artifactType {
	case types.MAVEN:
		return &MavenHandler{}
	case types.NPM:
		return &NpmHandler{}
	case types.PYTHON:
		return &PyPIHandler{}
	case types.NUGET:
		return &NuGetHandler{}
	case types.COMPOSER:
		return &ComposerHandler{}
	default:
		log.Debug().
			Str("artifactType", string(artifactType)).
			Msg("Using generic package handler")
		return &GenericHandler{}
	}
}

// GetPackageHandlerByGitLabType returns the appropriate handler for a GitLab package type
func GetPackageHandlerByGitLabType(packageType string) PackageHandler {
	switch strings.ToLower(packageType) {
	case "maven":
		return &MavenHandler{}
	case "npm":
		return &NpmHandler{}
	case "pypi":
		return &PyPIHandler{}
	case "nuget":
		return &NuGetHandler{}
	case "composer":
		return &ComposerHandler{}
	default:
		return &GenericHandler{}
	}
}
