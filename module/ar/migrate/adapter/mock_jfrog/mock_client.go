package mock_jfrog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	http2 "net/http"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

//go:embed testdata
var testdataFS embed.FS

type mockClient struct {
	registries    map[string]jfrog.JFrogRepository
	files         map[string][]types.File
	catalogs      map[string][]string
	fileContent   map[string][]byte // keyed by "registry/path"
	binaryContent map[string][]byte
}

// NewMockClient creates a mock implementation of jfrog.Client backed by
// embedded JSON fixture files in testdata/.
func NewMockClient() jfrog.Client {
	c := &mockClient{
		registries:    make(map[string]jfrog.JFrogRepository),
		files:         make(map[string][]types.File),
		catalogs:      make(map[string][]string),
		fileContent:   make(map[string][]byte),
		binaryContent: make(map[string][]byte),
	}
	c.loadRegistries()
	c.loadFiles()
	c.loadCatalogs()
	c.loadContent()
	c.loadBinaryContent()
	return c
}

func (c *mockClient) loadRegistries() {
	data, err := testdataFS.ReadFile("testdata/registries.json")
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &c.registries)
}

func (c *mockClient) loadFiles() {
	entries, err := testdataFS.ReadDir("testdata/files")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		registry := strings.TrimSuffix(name, ".json")
		data, err := testdataFS.ReadFile("testdata/files/" + name)
		if err != nil {
			continue
		}
		var files []types.File
		if err := json.Unmarshal(data, &files); err != nil {
			continue
		}
		c.files[registry] = files
	}
}

func (c *mockClient) loadCatalogs() {
	entries, err := testdataFS.ReadDir("testdata/catalogs")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		registry := strings.TrimSuffix(name, ".json")
		data, err := testdataFS.ReadFile("testdata/catalogs/" + name)
		if err != nil {
			continue
		}
		var repos []string
		if err := json.Unmarshal(data, &repos); err != nil {
			continue
		}
		c.catalogs[registry] = repos
	}
}

func (c *mockClient) loadContent() {
	_ = fs.WalkDir(testdataFS, "testdata/content", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := testdataFS.ReadFile(path)
		if err != nil {
			return nil
		}
		// path is like "testdata/content/maven-local/.pypi/simple.html"
		// we want the key "maven-local/.pypi/simple.html"
		key := strings.TrimPrefix(path, "testdata/content/")
		c.fileContent[key] = data
		return nil
	})
}

func (c *mockClient) loadBinaryContent() {
	// NPM package tarballs
	npmPackages := []struct {
		name, version, desc string
	}{
		{"@har/sample-package", "1.0.0", "A sample NPM package for testing migration"},
		{"@har/sample-package", "1.1.0", "A sample NPM package for testing migration"},
		{"@har/sample-package", "2.0.0", "A sample NPM package for testing migration"},
		{"@har/sample-package", "2.0.0-beta.1", "A sample NPM package beta version"},
		{"@har/sample-package", "3.0.0-rc.1", "A sample NPM package release candidate"},
		{"lodash", "4.17.21-alpha.0", "Lodash alpha version for testing"},
	}
	for _, p := range npmPackages {
		key := fmt.Sprintf("npm-local/%s/-/%s-%s.tgz", p.name, p.name, p.version)
		c.binaryContent[key] = createNpmPackageTgz(p.name, p.version, p.desc)
	}

	// Dart package tarballs
	dartPackages := []struct {
		name, version, desc string
	}{
		{"sample_dart_pkg", "1.0.0", "A sample Dart package for testing migration"},
		{"sample_dart_pkg", "1.1.0", "A sample Dart package for testing migration"},
		{"another_dart_pkg", "2.0.0", "Another Dart package for testing migration"},
	}
	for _, p := range dartPackages {
		key := fmt.Sprintf("dart-local/packages/%s/versions/%s.tar.gz", p.name, p.version)
		c.binaryContent[key] = createDartPackageTarGz(p.name, p.version, p.desc)
	}
}

func (c *mockClient) GetRegistries() ([]jfrog.JFrogRepository, error) {
	var repos []jfrog.JFrogRepository
	for _, repo := range c.registries {
		repos = append(repos, repo)
	}
	return repos, nil
}

func (c *mockClient) GetRegistry(registry string) (jfrog.JFrogRepository, error) {
	if repo, exists := c.registries[registry]; exists {
		return repo, nil
	}
	return jfrog.JFrogRepository{}, fmt.Errorf("registry %s not found", registry)
}

func (c *mockClient) GetFile(registry string, path string) (io.ReadCloser, http2.Header, error) {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	fileKey := fmt.Sprintf("%s/%s", registry, path)

	// Check text content from fixtures
	if content, exists := c.fileContent[fileKey]; exists {
		header := make(http2.Header)
		header.Set("Content-Type", "application/octet-stream")
		return io.NopCloser(bytes.NewReader(content)), header, nil
	}

	// Check binary content (tarballs)
	if content, exists := c.binaryContent[fileKey]; exists {
		header := make(http2.Header)
		header.Set("Content-Type", "application/gzip")
		return io.NopCloser(bytes.NewReader(content)), header, nil
	}

	return nil, nil, fmt.Errorf("file not found: %s", fileKey)
}

func (c *mockClient) GetFiles(registry string) ([]types.File, error) {
	repo, err := c.GetRegistry(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry %s: %w", registry, err)
	}
	if repo.Type == "VIRTUAL" {
		return nil, fmt.Errorf("registry %s is a virtual repository", registry)
	}

	if files, exists := c.files[registry]; exists {
		return files, nil
	}

	// Return default mock files if none configured
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
	}, nil
}

func (c *mockClient) GetCatalog(registry string) ([]string, error) {
	if catalog, exists := c.catalogs[registry]; exists {
		return catalog, nil
	}
	return []string{"mock-repo-1", "mock-repo-2", "sample-app"}, nil
}

// createDartPackageTarGz creates a valid tar.gz byte slice for a Dart package
func createDartPackageTarGz(packageName, version, description string) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	pubspecContent := fmt.Sprintf("name: %s\nversion: %s\ndescription: %s\n", packageName, version, description)

	files := []struct {
		name    string
		content string
	}{
		{"pubspec.yaml", pubspecContent},
	}

	for _, file := range files {
		hdr := &tar.Header{
			Name: file.name,
			Mode: 0644,
			Size: int64(len(file.content)),
		}
		tarWriter.WriteHeader(hdr)
		tarWriter.Write([]byte(file.content))
	}

	tarWriter.Close()
	gzWriter.Close()

	return buf.Bytes()
}

// createNpmPackageTgz creates a valid tar.gz byte slice for an NPM package
func createNpmPackageTgz(packageName, version, description string) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	packageJSON := fmt.Sprintf(`{
  "name": "%s",
  "version": "%s",
  "description": "%s",
  "main": "index.js",
  "license": "MIT"
}`, packageName, version, description)

	files := []struct {
		name    string
		content string
	}{
		{"package/package.json", packageJSON},
	}

	for _, file := range files {
		hdr := &tar.Header{
			Name: file.name,
			Mode: 0644,
			Size: int64(len(file.content)),
		}
		tarWriter.WriteHeader(hdr)
		tarWriter.Write([]byte(file.content))
	}

	tarWriter.Close()
	gzWriter.Close()

	return buf.Bytes()
}
