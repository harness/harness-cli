package mock_jfrog

import (
	"archive/tar"
	"archive/zip"
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
	// Try loading pre-generated content from testdata/binary/content/ first.
	// These are created by `make mock-init` and are not committed to git.
	loaded := 0
	_ = fs.WalkDir(testdataFS, "testdata/binary/content", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := testdataFS.ReadFile(path)
		if err != nil {
			return nil
		}
		// path is like "testdata/binary/content/maven-local/.pypi/simple.html"
		// we want the key "maven-local/.pypi/simple.html"
		key := strings.TrimPrefix(path, "testdata/binary/content/")
		c.fileContent[key] = data
		loaded++
		return nil
	})
	if loaded > 0 {
		return
	}

	// Fallback: generate content programmatically when testdata/binary/
	// doesn't exist (e.g. fresh clone without running `make mock-init`).
	c.fileContent["maven-local/.pypi/simple.html"] =
		[]byte(`<html><body><a href="requests/">requests</a><br/><a href="flask/">flask</a><br/></body></html>`)
	c.fileContent["maven-local/repodata/repomd.xml"] =
		[]byte(`<?xml version="1.0" encoding="UTF-8"?><repomd><data type="primary"><location href="repodata/primary.xml.gz"/></data></repomd>`)
	c.fileContent["maven-local/index.yaml"] =
		[]byte("apiVersion: v1\nentries:\n  nginx:\n    - name: nginx\n      version: 1.0.0\n      urls:\n        - charts/nginx-1.0.0.tgz\n")
	c.fileContent["helm-legacy-local/index.yaml"] =
		[]byte("apiVersion: v1\nentries:\n  nginx:\n    - name: nginx\n      version: 8.2.0\n      urls:\n        - nginx-8.2.0.tgz\n")
	c.fileContent["dart-local/sample_dart_pkg"] = []byte(`{
  "name": "sample_dart_pkg",
  "latest": {
    "version": "1.1.0",
    "pubspec": { "name": "sample_dart_pkg", "version": "1.1.0", "description": "A sample Dart package for testing migration", "homepage": "https://github.com/example/sample_dart_pkg", "environment": { "sdk": ">=2.17.0 <4.0.0" } },
    "archive_url": "http://localhost:8081/artifactory/dart-local/packages/sample_dart_pkg/versions/1.1.0.tar.gz"
  },
  "versions": [
    { "version": "1.0.0", "pubspec": { "name": "sample_dart_pkg", "version": "1.0.0", "description": "A sample Dart package for testing migration" }, "archive_url": "http://localhost:8081/artifactory/dart-local/packages/sample_dart_pkg/versions/1.0.0.tar.gz" },
    { "version": "1.1.0", "pubspec": { "name": "sample_dart_pkg", "version": "1.1.0", "description": "A sample Dart package for testing migration" }, "archive_url": "http://localhost:8081/artifactory/dart-local/packages/sample_dart_pkg/versions/1.1.0.tar.gz" }
  ]
}`)
	c.fileContent["npm-local/@har/sample-package"] = []byte(`{
  "name": "@har/sample-package",
  "description": "A sample NPM package for testing migration",
  "dist-tags": { "latest": "2.0.0", "beta": "2.0.0" },
  "versions": {
    "1.0.0": { "name": "@har/sample-package", "version": "1.0.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-1.0.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80707" } },
    "1.1.0": { "name": "@har/sample-package", "version": "1.1.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-1.1.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80711" } },
    "2.0.0": { "name": "@har/sample-package", "version": "2.0.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-2.0.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80712" } },
    "2.0.0-beta.1": { "name": "@har/sample-package", "version": "2.0.0-beta.1", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-2.0.0-beta.1.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80713" } },
    "3.0.0-rc.1": { "name": "@har/sample-package", "version": "3.0.0-rc.1", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-3.0.0-rc.1.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80714" } }
  },
  "time": { "created": "2023-01-01T00:00:00.000Z", "modified": "2023-05-01T00:00:00.000Z" }
}`)
	c.fileContent["npm-local/lodash"] = []byte(`{
  "name": "lodash",
  "description": "Lodash modular utilities",
  "dist-tags": { "latest": "4.17.21", "alpha": "4.17.21-alpha.0" },
  "versions": {
    "4.17.21-alpha.0": { "name": "lodash", "version": "4.17.21-alpha.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/lodash/-/lodash-4.17.21-alpha.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80715" } }
  },
  "time": { "created": "2023-06-01T00:00:00.000Z", "modified": "2023-06-01T00:00:00.000Z" }
}`)
	c.fileContent["python-local/.pypi/simple.html"] =
		[]byte(`<html><body><a href="requests/">requests</a><br/></body></html>`)
	c.fileContent["python-local/.pypi/requests/requests.html"] =
		[]byte(`<html><body><a href="../requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a><br/><a href="../requests-2.29.0.tar.gz#sha256=def456">requests-2.29.0.tar.gz</a><br/></body></html>`)
}

func (c *mockClient) loadBinaryContent() {
	// Try loading pre-generated binaries from testdata/binary/ first.
	// These are created by `make mock-init` and are not committed to git.
	loaded := 0
	_ = fs.WalkDir(testdataFS, "testdata/binary", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := testdataFS.ReadFile(path)
		if err != nil {
			return nil
		}
		key := strings.TrimPrefix(path, "testdata/binary/")
		c.binaryContent[key] = data
		loaded++
		return nil
	})
	if loaded > 0 {
		return
	}

	// Fallback: generate binaries programmatically when testdata/binary/
	// doesn't exist (e.g. fresh clone without running `make mock-init`).
	npmPkgs := []struct{ name, version string }{
		{"@har/sample-package", "1.0.0"},
		{"@har/sample-package", "1.1.0"},
		{"@har/sample-package", "2.0.0"},
		{"@har/sample-package", "2.0.0-beta.1"},
		{"@har/sample-package", "3.0.0-rc.1"},
		{"lodash", "4.17.21-alpha.0"},
	}
	for _, p := range npmPkgs {
		key := fmt.Sprintf("npm-local/%s/-/%s-%s.tgz", p.name, p.name, p.version)
		c.binaryContent[key] = createNpmPackageTgz(p.name, p.version, "mock")
	}

	dartPkgs := []struct{ name, version string }{
		{"sample_dart_pkg", "1.0.0"},
		{"sample_dart_pkg", "1.1.0"},
		{"another_dart_pkg", "2.0.0"},
	}
	for _, p := range dartPkgs {
		key := fmt.Sprintf("dart-local/packages/%s/versions/%s.tar.gz", p.name, p.version)
		c.binaryContent[key] = createDartPackageTarGz(p.name, p.version, "mock")
	}

	nugetPkgs := []struct{ id, version string }{
		{"company.grpc.pkg", "1.0.0"},
		{"company.grpc.pkg", "2.0.0"},
	}
	for _, p := range nugetPkgs {
		key := fmt.Sprintf("nuget-local/foo/%s/%s/%s.%s.nupkg", p.id, p.version, p.id, p.version)
		c.binaryContent[key] = createNugetPackageNupkg(p.id, p.version)
	}
	c.binaryContent["nuget-local/foo/company.grpc.pkg/2.0.0/company.grpc.pkg.2.0.0.snupkg"] =
		createNugetPackageNupkg("company.grpc.pkg", "2.0.0")
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

// createNugetPackageNupkg creates a minimal valid .nupkg (ZIP with a .nuspec).
func createNugetPackageNupkg(id, version string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	nuspec := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <id>%s</id>
    <version>%s</version>
    <authors>test</authors>
    <description>Mock NuGet package for migration testing</description>
  </metadata>
</package>`, id, version)

	f, _ := w.Create(id + ".nuspec")
	f.Write([]byte(nuspec))

	w.Close()
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
