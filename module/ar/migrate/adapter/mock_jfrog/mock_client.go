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
	searchedFiles map[string][]types.SearchedFile
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
		searchedFiles: make(map[string][]types.SearchedFile),
		catalogs:      make(map[string][]string),
		fileContent:   make(map[string][]byte),
		binaryContent: make(map[string][]byte),
	}
	c.loadRegistries()
	c.loadFiles()
	c.loadSearchedFiles()
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

func (c *mockClient) loadSearchedFiles() {
	entries, err := testdataFS.ReadDir("testdata/search")
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
		data, err := testdataFS.ReadFile("testdata/search/" + name)
		if err != nil {
			continue
		}
		type aqlResponse struct {
			Results []types.SearchedFile `json:"results"`
		}
		var resp aqlResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		c.searchedFiles[registry] = resp.Results
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
	// HELM_HTTP index lists only the flat nginx chart (with a relative URL, the
	// JFrog default since Artifactory 7.59.5). The nested abc chart and the
	// orphan chart are intentionally absent so the hybrid tree sweep recovers
	// them — exercising index+tree dedup.
	c.fileContent["helm-http-local/index.yaml"] =
		[]byte("apiVersion: v1\nentries:\n  nginx:\n    - name: nginx\n      version: 1.0.0\n      urls:\n        - nginx-1.0.0.tgz\n")
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
	fmt.Printf("DEBUG: Loaded %d file content entries\n", len(c.fileContent))
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

	puppetPkgs := []struct{ author, module, version string }{
		{"puppetlabs", "stdlib", "9.4.1"},
		{"puppetlabs", "stdlib", "9.5.0"},
		{"puppetlabs", "apache", "12.3.0"},
	}
	for _, p := range puppetPkgs {
		key := fmt.Sprintf("puppet-local/%s/%s/%s-%s-%s.tar.gz", p.author, p.module, p.author, p.module, p.version)
		c.binaryContent[key] = createPuppetPackageTarGz(p.author, p.module, p.version)
	}

	// HELM_HTTP fixtures. The download key for a chart is "registry/<chart-uri>"
	// where <chart-uri> is what enumeration resolves: for index entries it is the
	// (relative) index URL, for tree-sweep entries it is the file Uri (which may
	// carry a nested directory prefix). The leaf chart name (last path segment,
	// minus -<version>.tgz) must match the embedded Chart.yaml so HAR's
	// putHelmChartFile metadata check passes.
	//
	//   nginx-1.0.0.tgz                 → in index.yaml (flat, relative URL)
	//   nginx-1.0.0.tgz.prov            → sibling provenance of the indexed chart
	//   ChartA/ChartB/abc-1.0.1.tgz     → on disk only (hybrid tree sweep), nested
	//   orphan-2.0.0.tgz                → on disk only (hybrid tree sweep), flat
	//   team-a/abc-1.0.1.tgz, team-b/abc-1.0.1.tgz → distinct nested charts that
	//                                  share a leaf name+version (collision case)
	helmHTTPCharts := []struct{ key, leaf, version string }{
		{"helm-http-local/nginx-1.0.0.tgz", "nginx", "1.0.0"},
		{"helm-http-local/ChartA/ChartB/abc-1.0.1.tgz", "abc", "1.0.1"},
		{"helm-http-local/orphan-2.0.0.tgz", "orphan", "2.0.0"},
		{"helm-http-local/team-a/abc-1.0.1.tgz", "abc", "1.0.1"},
		{"helm-http-local/team-b/abc-1.0.1.tgz", "abc", "1.0.1"},
	}
	for _, ch := range helmHTTPCharts {
		c.binaryContent[ch.key] = createHelmChartTgz(ch.leaf, ch.version)
	}
	// Provenance sidecar — opaque bytes; the server treats it as a blob attached
	// to the already-uploaded chart.
	c.binaryContent["helm-http-local/nginx-1.0.0.tgz.prov"] =
		[]byte("-----BEGIN PGP SIGNED MESSAGE-----\nmock provenance for nginx-1.0.0\n-----END PGP SIGNATURE-----\n")

	// Conan v2 files — opaque bytes keyed by their repo-relative Uri. Enumeration
	// derives the download key from the file Uri, so every canonical file listed
	// in conan-local.json needs matching content here.
	conanFiles := []string{
		"zlib/1.2.13/_/_/9a0b1c2d3e4f5061728394a5b6c7d8e9/export/conanfile.py",
		"zlib/1.2.13/_/_/9a0b1c2d3e4f5061728394a5b6c7d8e9/export/conan_export.tgz",
		"zlib/1.2.13/_/_/9a0b1c2d3e4f5061728394a5b6c7d8e9/export/conanmanifest.txt",
		"zlib/1.2.13/_/_/9a0b1c2d3e4f5061728394a5b6c7d8e9/package/abcabcabcabcabcabcabcabcabcabcabcabcabca/1f2e3d4c5b6a7988990a1b2c3d4e5f60/conaninfo.txt",
		"zlib/1.2.13/_/_/9a0b1c2d3e4f5061728394a5b6c7d8e9/package/abcabcabcabcabcabcabcabcabcabcabcabcabca/1f2e3d4c5b6a7988990a1b2c3d4e5f60/conan_package.tgz",
		"zlib/1.2.13/_/_/9a0b1c2d3e4f5061728394a5b6c7d8e9/package/abcabcabcabcabcabcabcabcabcabcabcabcabca/1f2e3d4c5b6a7988990a1b2c3d4e5f60/conanmanifest.txt",
		"mylib/2.0/acme/stable/0011223344556677889900aabbccddee/export/conanfile.py",
		"mylib/2.0/acme/stable/0011223344556677889900aabbccddee/export/conanmanifest.txt",
	}
	for _, uri := range conanFiles {
		c.binaryContent["conan-local/"+uri] = []byte("mock conan content: " + uri)
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

	// Debug: print available keys for this registry
	fmt.Printf("DEBUG: File not found: %s\n", fileKey)
	fmt.Printf("DEBUG: Available file content keys for %s:\n", registry)
	for k := range c.fileContent {
		if strings.HasPrefix(k, registry+"/") {
			fmt.Printf("  - %s\n", k)
		}
	}
	fmt.Printf("DEBUG: Available binary content keys for %s:\n", registry)
	for k := range c.binaryContent {
		if strings.HasPrefix(k, registry+"/") {
			fmt.Printf("  - %s\n", k)
		}
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
func (c *mockClient) SearchFiles(registry string) ([]types.SearchedFile, error) {
	if files, exists := c.searchedFiles[registry]; exists {
		return files, nil
	}
	return nil, fmt.Errorf("no search data found for registry '%s'", registry)
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

// createPuppetPackageTarGz creates a minimal valid Puppet module .tar.gz
// containing a metadata.json at the expected "<author>-<module>/metadata.json"
// path. AR's puppet upload handler reads metadata.json from this depth.
func createPuppetPackageTarGz(author, module, version string) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	moduleDir := fmt.Sprintf("%s-%s", author, module)
	metadata := fmt.Sprintf(`{
  "name": "%s-%s",
  "version": "%s",
  "author": "%s",
  "summary": "Mock Puppet module for migration testing",
  "license": "Apache-2.0",
  "source": "https://example.com/%s/%s",
  "dependencies": []
}`, author, module, version, author, author, module)

	files := []struct {
		name    string
		content string
	}{
		{moduleDir + "/metadata.json", metadata},
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

// createHelmChartTgz creates a valid Helm chart .tgz containing a Chart.yaml at
// "<name>/Chart.yaml" whose name/version match the arguments. HAR's
// putHelmChartFile opens the archive, reads Chart.yaml, and rejects the upload
// if name/version disagree with the parsed file name, so the embedded metadata
// must equal the leaf chart name/version.
func createHelmChartTgz(name, version string) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	chartYaml := fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\ndescription: Mock Helm chart for migration testing\ntype: application\n",
		name, version)

	files := []struct {
		name    string
		content string
	}{
		{name + "/Chart.yaml", chartYaml},
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
