package mock_jfrog

import (
	"bytes"
	"fmt"
	"io"
	http2 "net/http"
	"os"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog/log"
)

// newClient constructs a mock jfrog client
func newClient(reg *types.RegistryConfig) *client {
	return &client{
		url:      reg.Endpoint,
		username: reg.Credentials.Username,
		password: reg.Credentials.Password,
		mockData: initMockData(),
	}
}

type client struct {
	url      string
	username string
	password string
	mockData *mockData
}

// mockData holds all the mock responses
type mockData struct {
	registries  map[string]JFrogRepository
	files       map[string][]types.File
	catalogs    map[string][]string
	fileContent map[string]string
}

// JFrogPackage represents a file entry from JFrog Artifactory
type JFrogPackage struct {
	Registry string
	Path     string
	Name     string
	Size     int
}

type JFrogRepository struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Url         string `json:"url"`
	Description string `json:"description"`
	PackageType string `json:"packageType"`
}

func (c *client) getRegistries() ([]JFrogRepository, error) {
	// Return mock repositories
	var repositories []JFrogRepository
	for _, repo := range c.mockData.registries {
		repositories = append(repositories, repo)
	}
	return repositories, nil
}

func (c *client) getRegistry(registry string) (JFrogRepository, error) {
	// Return mock registry data
	if repo, exists := c.mockData.registries[registry]; exists {
		return repo, nil
	}
	return JFrogRepository{}, fmt.Errorf("registry %s not found", registry)
}

func (c *client) getFile(registry string, path string) (io.ReadCloser, http2.Header, error) {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	// Create file key for lookup
	fileKey := fmt.Sprintf("%s/%s", registry, path)

	// Return mock file content
	if content, exists := c.mockData.fileContent[fileKey]; exists {
		header := make(http2.Header)
		header.Set("Content-Type", "application/octet-stream")
		return io.NopCloser(bytes.NewReader([]byte(content))), header, nil
	}

	if strings.HasPrefix(path, "tmp/") || strings.HasPrefix(path, "Users/") {
		file, err := os.Open("/" + path)
		if err == nil {
			header := make(http2.Header)
			header.Set("Content-Type", "application/octet-stream")
			return file, header, nil
		}

		log.Error().Err(err).Str("path", path).Str("registry", registry).Msgf("failed to read file")

	}

	// Return default mock content for common files
	defaultContent := c.getDefaultFileContent(path)
	header := make(http2.Header)
	header.Set("Content-Type", "application/octet-stream")
	return io.NopCloser(bytes.NewReader([]byte(defaultContent))), header, nil
}

// getFiles retrieves a list of mock files from the specified registry
func (c *client) getFiles(registry string) ([]types.File, error) {
	repo, err := c.getRegistry(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry %s: %w", registry, err)
	}
	if repo.Type == "VIRTUAL" {
		return nil, fmt.Errorf("registry %s is a virtual repository", registry)
	}

	// Return mock files for the registry
	if files, exists := c.mockData.files[registry]; exists {
		return files, nil
	}

	// Return default mock files if none configured
	return c.getDefaultFiles(registry), nil
}

func getFileName(uri string) string {
	// Normalize the URI by removing any leading/trailing slashes
	uri = strings.TrimPrefix(uri, "/")
	uri = strings.TrimSuffix(uri, "/")

	// Handle empty URI
	if uri == "" {
		return ""
	}

	// Split the URI by path separator
	parts := strings.Split(uri, "/")

	// Return the last part, which should be the filename
	return parts[len(parts)-1]
}

func (c *client) getCatalog(registry string) (repositories []string, err error) {
	// Return mock catalog data
	if catalog, exists := c.mockData.catalogs[registry]; exists {
		return catalog, nil
	}

	// Return default mock catalog
	return []string{"mock-repo-1", "mock-repo-2", "sample-app"}, nil
}

// initMockData initializes the mock data structures
func initMockData() *mockData {
	return &mockData{
		registries: map[string]JFrogRepository{
			"docker-local": {
				Key:         "docker-local",
				Type:        "LOCAL",
				Url:         "http://localhost:8081/artifactory/docker-local",
				Description: "Mock Docker Local Repository",
				PackageType: "Docker",
			},
			"helm-legacy-local": {
				Key:         "helm-legacy-local",
				Type:        "LOCAL",
				Url:         "http://localhost:8081/artifactory/helm-legacy-local",
				Description: "Mock helm-legacy Local Repository",
				PackageType: "HELM",
			},
			"maven-local": {
				Key:         "maven-local",
				Type:        "LOCAL",
				Url:         "http://localhost:8081/artifactory/maven-local",
				Description: "Mock Maven Local Repository",
				PackageType: "Maven",
			},
			"npm-local": {
				Key:         "npm-local",
				Type:        "LOCAL",
				Url:         "http://localhost:8081/artifactory/npm-local",
				Description: "Mock NPM Local Repository",
				PackageType: "npm",
			},
			"generic-local": {
				Key:         "generic-local",
				Type:        "LOCAL",
				Url:         "http://localhost:8081/artifactory/generic-local",
				Description: "Mock Generic Local Repository",
				PackageType: "Generic",
			},
		},
		files: map[string][]types.File{
			"maven-local": {
				{
					Registry:     "maven-local",
					Name:         "sample-1.0.jar",
					Uri:          "/com/example/sample/1.0/sample-1.0.jar",
					Folder:       false,
					Size:         1024,
					LastModified: "2023-01-01T00:00:00.000Z",
					SHA1:         "da39a3ee5e6b4b0d3255bfef95601890afd80709",
				},
				{
					Registry:     "maven-local",
					Name:         "sample-1.0.pom",
					Uri:          "/com/example/sample/1.0/sample-1.0.pom",
					Folder:       false,
					Size:         512,
					LastModified: "2023-01-01T00:00:00.000Z",
					SHA1:         "da39a3ee5e6b4b0d3255bfef95601890afd80708",
				},
			},
			"npm-local": {
				{
					Registry:     "npm-local",
					Name:         "sample-package-1.0.0.tgz",
					Uri:          "/sample-package/-/sample-package-1.0.0.tgz",
					Folder:       false,
					Size:         2048,
					LastModified: "2023-01-01T00:00:00.000Z",
					SHA1:         "da39a3ee5e6b4b0d3255bfef95601890afd80707",
				},
			},
			"helm-legacy-local": {
				{
					Registry:     "helm-legacy-local",
					Name:         "nginx-8.2.0.tgz",
					Uri:          "/nginx-8.2.0.tgz",
					Folder:       false,
					Size:         2048,
					LastModified: "2023-01-01T00:00:00.000Z",
					SHA1:         "da39a3ee5e6b4b0d3255bfef95601890afd80707",
				},
				{
					Registry:     "index.yaml",
					Name:         "index.yaml",
					Uri:          "/index.yaml",
					Folder:       false,
					Size:         2048,
					LastModified: "2023-01-01T00:00:00.000Z",
					SHA1:         "da39a3ee5e6b4b0d3255bfef95601890afd80707",
				},
			},
		},
		catalogs: map[string][]string{
			"docker-local": {"sample-app", "nginx", "alpine"},
		},
		fileContent: map[string]string{
			"maven-local/.pypi/simple.html":   `<html><body><a href="requests/">requests</a><br/><a href="flask/">flask</a><br/></body></html>`,
			"maven-local/repodata/repomd.xml": `<?xml version="1.0" encoding="UTF-8"?><repomd><data type="primary"><location href="repodata/primary.xml.gz"/></data></repomd>`,
			"maven-local/index.yaml":          `apiVersion: v1\nentries:\n  nginx:\n    - name: nginx\n      version: 1.0.0\n      urls:\n        - charts/nginx-1.0.0.tgz`,
			"npm-local/sample-package": `{
  "name": "sample-package",
  "description": "A sample NPM package for testing migration",
  "dist-tags": {
    "latest": "2.0.0",
    "beta" : "2.0.0"
  },
  "versions": {
    "1.0.0": {
      "name": "sample-package",
      "version": "1.0.0",
      "description": "A sample NPM package for testing migration",
      "main": "index.js",
      "scripts": {
        "test": "echo \"Error: no test specified\" && exit 1"
      },
      "keywords": ["sample", "testing", "migration"],
      "author": "Test Author",
      "license": "MIT",
      "dist": {
        "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-1.0.0.tgz",
        "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80707"
      }
    },
    "1.1.0": {
      "name": "sample-package",
      "version": "1.1.0",
      "description": "A sample NPM package for testing migration",
      "main": "index.js",
      "scripts": {
        "test": "echo \"Error: no test specified\" && exit 1"
      },
      "keywords": ["sample", "testing", "migration"],
      "author": "Test Author",
      "license": "MIT",
      "dist": {
        "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-1.1.0.tgz",
        "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80711"
      }
    },
    "2.0.0": {
      "name": "sample-package",
      "version": "2.0.0",
      "description": "A sample NPM package for testing migration",
      "main": "index.js",
      "scripts": {
        "test": "echo \"Error: no test specified\" && exit 1"
      },
      "keywords": ["sample", "testing", "migration"],
      "author": "Test Author",
      "license": "MIT",
      "dist": {
        "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-2.0.0.tgz",
        "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80712"
      }
    }
  },
  "time": {
    "created": "2023-01-01T00:00:00.000Z",
    "modified": "2023-03-01T00:00:00.000Z",
    "1.0.0": "2023-01-01T00:00:00.000Z",
    "1.1.0": "2023-02-01T00:00:00.000Z",
    "2.0.0": "2023-03-01T00:00:00.000Z"
  }
}`,
		},
	}
}

// getDefaultFileContent returns default content for common file types
func (c *client) getDefaultFileContent(path string) string {
	if strings.HasSuffix(path, ".html") {
		return `<html><body><h1>Mock HTML Content</h1></body></html>`
	}
	if strings.HasSuffix(path, ".xml") {
		return `<?xml version="1.0"?><root><mock>data</mock></root>`
	}
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return `mock: data\nversion: 1.0.0`
	}
	if strings.HasSuffix(path, ".json") {
		return `{"mock": "data", "version": "1.0.0"}`
	}
	return "mock file content"
}

// extractPackageNameFromPath extracts the package name from a .tgz file path
func extractPackageNameFromPath(path string) string {
	// Extract filename from path
	parts := strings.Split(path, "/")
	filename := parts[len(parts)-1]

	// Remove .tgz extension
	if strings.HasSuffix(filename, ".tgz") {
		filename = strings.TrimSuffix(filename, ".tgz")
	}

	// Extract package name based on pattern
	if strings.HasPrefix(filename, "@") {
		// For scoped packages like @angular-core-15.2.1
		parts := strings.Split(filename, "-")
		if len(parts) >= 3 {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	} else {
		// For regular packages like lodash-4.17.21
		lastHyphenIndex := strings.LastIndex(filename, "-")
		if lastHyphenIndex > 0 {
			return filename[:lastHyphenIndex]
		}
	}

	return filename
}

// extractVersionFromPath extracts the version from a .tgz file path
func extractVersionFromPath(path string) string {
	// Extract filename from path
	parts := strings.Split(path, "/")
	filename := parts[len(parts)-1]

	// Remove .tgz extension
	if strings.HasSuffix(filename, ".tgz") {
		filename = strings.TrimSuffix(filename, ".tgz")
	}

	// Extract version based on pattern
	if strings.HasPrefix(filename, "@") {
		// For scoped packages like @angular-core-15.2.1
		parts := strings.Split(filename, "-")
		if len(parts) >= 3 {
			return parts[len(parts)-1]
		}
	} else {
		// For regular packages like lodash-4.17.21
		lastHyphenIndex := strings.LastIndex(filename, "-")
		if lastHyphenIndex > 0 {
			return filename[lastHyphenIndex+1:]
		}
	}

	return "1.0.0" // default version
}

// getDefaultFiles returns default mock files for a registry
func (c *client) getDefaultFiles(registry string) []types.File {
	return []types.File{
		{
			Registry:     registry,
			Name:         "mock-file.txt",
			Uri:          "/mock-file.txt",
			Folder:       false,
			Size:         100,
			LastModified: "2023-01-01T00:00:00.000Z",
			SHA1:         "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
	}
}

func buildCatalogURL(endpoint, repo string) string {
	return fmt.Sprintf("%s/artifactory/api/docker/%s/v2/_catalog?n=1000", endpoint, repo)
}
