package mock_jfrog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	mavenMetadataFile = "maven-metadata.xml"
	extensionMD5      = ".md5"
	extensionSHA1     = ".sha1"
	extensionSHA256   = ".sha256"
	extensionSHA512   = ".sha512"
	extensionPom      = ".pom"
	extensionJar      = ".jar"
	contentTypeJar    = "application/java-archive"
	contentTypeXML    = "text/xml"
)

func init() {
	adapterType := types.MOCK_JFROG
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

// factory section
type factory struct {
}

// Create an adapter section
func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	return newAdapter(config)
}

type adapter struct {
	client *client
	reg    types.RegistryConfig
}

func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	return &adapter{
		client: newClient(&config),
		reg:    config,
	}, nil
}

func (a *adapter) GetKeyChain(reg string) authn.Keychain {
	host, _ := dockerHost(a.reg.Endpoint, reg)
	return NewJfrogKeychain(a.reg.Credentials.Username, a.reg.Credentials.Password, host)
}

func (a *adapter) GetConfig() types.RegistryConfig {
	return a.reg
}

func (a *adapter) ValidateCredentials() (bool, error) {
	// Mock always returns true for valid credentials
	return true, nil
}
func (a *adapter) GetRegistry(ctx context.Context, registry string) (types.RegistryInfo, error) {
	reg, err := a.client.getRegistry(registry)
	if err != nil {
		return types.RegistryInfo{}, fmt.Errorf("get registry: %w", err)
	}
	return types.RegistryInfo{
		Type: reg.Type,
		URL:  reg.Url,
	}, nil
}
func (a *adapter) CreateRegistryIfDoesntExist(registry string) (bool, error) {
	// Mock always returns true (registry "created")
	return true, nil
}

func (a *adapter) GetPackages(registry string, artifactType types.ArtifactType, root *types.TreeNode) (
	[]types.Package,
	error,
) {
	var packages []types.Package
	if artifactType == types.GENERIC {
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "default",
			Size:     -1,
		})
	} else if artifactType == types.MAVEN || artifactType == types.NUGET {
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "",
			Size:     -1,
		})
	} else if artifactType == types.DOCKER || artifactType == types.HELM {
		catalog, err := a.client.getCatalog(registry)
		if err != nil {
			return nil, fmt.Errorf("get catalog: %w", err)
		}

		for _, repo := range catalog {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     repo,
				Size:     -1,
			})
		}
	} else if artifactType == types.HELM_LEGACY {
		_, err := tree.GetNodeForPath(root, "/index.yaml")
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}

		// Generate dynamic index content from charts path
		// You can set the charts path here or make it configurable
		chartsPath := "/Users/arvindchoudary/Work/helm-charts/mysql_all/charts" // Default path, can be made configurable

		var indexContent string
		if _, err := os.Stat(chartsPath); os.IsNotExist(err) {
			// Fallback to static mock data if charts path doesn't exist
			log.Warn().Msgf("Charts path %s does not exist, using static mock data", chartsPath)
			indexContent = `apiVersion: v1
entries:
  nginx:
    - name: nginx
      version: 8.2.0
      urls:
        - tmp/nginx-8.2.0.tgz`
		} else {
			// Use dynamic generation from charts path
			generatedContent, err := GenerateHelmIndexFromPath(chartsPath)
			if err != nil {
				log.Warn().Msgf("Failed to generate dynamic index content: %v, using static mock data", err)
				indexContent = `apiVersion: v1
entries:
  nginx:
    - name: nginx
      version: 8.2.0
      urls:
        - tmp/nginx-8.2.0.tgz`
			} else {
				indexContent = generatedContent
			}
		}

		tmp, err := os.CreateTemp("", "index-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tmp.Name())
		_, err = tmp.WriteString(indexContent)
		if err != nil {
			return nil, fmt.Errorf("write index content: %w", err)
		}
		tmp.Close()
		index, err := repo.LoadIndexFile(tmp.Name())
		if err != nil {
			return nil, fmt.Errorf("load index file: %w", err)
		}

		for name, entries := range index.Entries {
			for _, ver := range entries {
				packages = append(packages, types.Package{
					Registry: registry,
					Path:     "/",
					Name:     name,
					Size:     -1,
					URL:      ver.URLs[0],
					Version:  ver.Version,
				})
			}
		}
	} else if artifactType == types.PYTHON {
		_, err := tree.GetNodeForPath(root, "/.pypi/simple.html")
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}
		// Use mock data for Python packages instead of downloading
		mockPythonHTML := `<html><body><a href="requests/">requests</a><br/><a href="flask/">flask</a><br/><a href="django/">django</a><br/></body></html>`
		file := io.NopCloser(strings.NewReader(mockPythonHTML))
		defer file.Close()
		_packages, err := extractPythonPackageNames(file)
		if err != nil {
			return nil, fmt.Errorf("extract python package names: %w", err)
		}
		for _, p := range _packages {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     p,
				Size:     -1,
			})
		}

		return packages, nil
	} else if artifactType == types.RPM {
		_, err := tree.GetNodeForPath(root, "/repodata/repomd.xml")
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}
		// Use mock data for RPM repomd.xml instead of downloading
		mockRepomdXML := `<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo" xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <data type="primary">
    <location href="repodata/primary.xml.gz"/>
  </data>
</repomd>`
		file := io.NopCloser(strings.NewReader(mockRepomdXML))
		defer file.Close()

		_, err = extractPrimaryLocation(file)
		if err != nil {
			return nil, fmt.Errorf("extract primary location: %w", err)
		}

		// Use mock data for RPM primary.xml.gz instead of downloading
		mockPrimaryXML := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="2">
  <package type="rpm">
    <name>sample-package</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.0.0" rel="1"/>
    <size package="1024" installed="2048" archive="512"/>
    <location href="sample-package-1.0.0-1.x86_64.rpm"/>
  </package>
  <package type="rpm">
    <name>another-package</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="2.0.0" rel="1"/>
    <size package="2048" installed="4096" archive="1024"/>
    <location href="another-package-2.0.0-1.x86_64.rpm"/>
  </package>
</metadata>`
		// Create a mock gzipped content
		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		_, err = gzWriter.Write([]byte(mockPrimaryXML))
		if err != nil {
			return nil, fmt.Errorf("write gzip content: %w", err)
		}
		gzWriter.Close()
		primaryFile := io.NopCloser(bytes.NewReader(buf.Bytes()))
		defer primaryFile.Close()

		// Extract package URLs from primary.xml.gz
		packages, err := extractRPMPackages(primaryFile, registry)
		if err != nil {
			return nil, fmt.Errorf("extract RPM package URLs: %w", err)
		}
		return packages, nil
	} else if artifactType == types.GO {
		leaves, _ := tree.GetAllFiles(root)
		packageMap := make(map[string]bool)
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}
			if !strings.Contains(leaf.Uri, "/@v/") {
				continue
			}
			pkgName := strings.Split(leaf.Uri, "/@v/")[0]
			path := "/"
			if _, ok := packageMap[pkgName]; ok {
				continue
			}
			packageMap[pkgName] = true
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     path,
				Name:     strings.TrimPrefix(pkgName, path),
				Size:     leaf.Size,
			})
		}
		return packages, nil
	} else if artifactType == types.NPM {
		leaves, _ := tree.GetAllFiles(root)
		packageMap := make(map[string]bool)
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}
			if !strings.Contains(leaf.Uri, ".tgz") {
				continue
			}
			// For NPM packages, extract package name from URI path instead of filename
			// URI structure: /@harness/sample-package/-/@harness/sample-package-1.0.0.tgz
			filename := leaf.Name
			if !strings.HasSuffix(filename, ".tgz") {
				continue
			}

			// Extract package name from URI path before "/-/" delimiter
			var pkgName string
			uri := leaf.Uri
			if idx := strings.Index(uri, "/-/"); idx != -1 {
				// Get the path before "/-/"
				pathBeforeDelimiter := uri[:idx]
				// Remove leading slash if present
				if strings.HasPrefix(pathBeforeDelimiter, "/") {
					pathBeforeDelimiter = pathBeforeDelimiter[1:]
				}
				pkgName = pathBeforeDelimiter
			} else {
				// Fallback to extracting from filename if URI doesn't contain "/-/"
				nameWithVersion := strings.TrimSuffix(filename, ".tgz")
				if strings.HasPrefix(nameWithVersion, "@") {
					// For scoped packages like @scope-package-name-version
					parts := strings.Split(nameWithVersion, "-")
					if len(parts) >= 3 {
						pkgName = strings.Join(parts[:len(parts)-1], "-")
					} else {
						pkgName = nameWithVersion
					}
				} else {
					// For regular packages like package-name-version
					lastHyphenIndex := strings.LastIndex(nameWithVersion, "-")
					if lastHyphenIndex > 0 {
						pkgName = nameWithVersion[:lastHyphenIndex]
					} else {
						pkgName = nameWithVersion
					}
				}
			}
			path := "/"
			if _, ok := packageMap[pkgName]; ok {
				continue
			}
			packageMap[pkgName] = true
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     path,
				Name:     pkgName,
				Size:     leaf.Size,
			})
		}
		return packages, nil
	} else {
		return []types.Package{}, errors.New("unknown artifact type")
	}

	return packages, nil
}

func extractPythonPackageNames(r io.Reader) ([]string, error) {
	var pkgs []string

	z := html.NewTokenizer(r)
	for {
		switch z.Next() {

		case html.ErrorToken:
			if z.Err() == io.EOF {
				return pkgs, nil // finished parsing
			}
			return nil, z.Err() // real error

		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data != "a" {
				continue
			}
			for _, attr := range tok.Attr {
				if attr.Key == "href" {
					pkgs = append(pkgs, attr.Val)
				}
			}
		}
	}
}

func resolveHref(basePath, href string) string {
	baseDir := path.Dir(basePath) // -> "start/foo/bar"
	return path.Clean(path.Join(baseDir, href))
	// Clean collapses ../, ./, and duplicate slashes.
}

func extractPrimaryLocation(file io.Reader) (string, error) {
	var repomd repomdData
	if err := xml.NewDecoder(file).Decode(&repomd); err != nil {
		return "", fmt.Errorf("parse repomd.xml: %w", err)
	}

	// Find the data element with type="primary"
	for _, data := range repomd.Data {
		if data.Type == "primary" {
			return data.Location.Href, nil
		}
	}

	return "", fmt.Errorf("primary.xml.gz location not found in repomd.xml")
}

func extractRPMPackages(file io.Reader, registry string) ([]types.Package, error) {
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gz.Close()

	decoder := xml.NewDecoder(gz)
	var packages []types.Package

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse XML: %w", err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Local == "package" {
			var pkg primaryPackage
			if err := decoder.DecodeElement(&pkg, &start); err != nil {
				return nil, fmt.Errorf("decode package: %w", err)
			}

			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     path.Base(pkg.Location.Href),
				URL:      pkg.Location.Href,
				Size:     pkg.Size.Package,
			})
		}
	}

	return packages, nil
}

func (a *adapter) GetVersions(
	p types.Package,
	node *types.TreeNode,
	registry, pkg string,
	artifactType types.ArtifactType,
) ([]types.Version, error) {
	if artifactType == types.GENERIC {
		return []types.Version{
			{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     "default",
				Size:     -1,
			},
		}, nil
	}

	if artifactType == types.MAVEN || artifactType == types.NUGET {
		return []types.Version{
			{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     "",
				Size:     -1,
			},
		}, nil
	}

	if artifactType == types.HELM_LEGACY {
		return []types.Version{
			{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     "",
				Size:     -1,
			},
		}, nil
	}

	if artifactType == types.PYTHON {
		var versions []types.Version
		indexPath := fmt.Sprintf(".pypi/%s/%s.html", pkg, pkg)
		// Use mock data for Python package versions instead of downloading
		mockPackageHTML := fmt.Sprintf(`<html><body><a href="%s-1.0.0.tar.gz">%s-1.0.0.tar.gz</a><br/><a href="%s-1.1.0.tar.gz">%s-1.1.0.tar.gz</a><br/></body></html>`,
			pkg, pkg, pkg, pkg)
		file := io.NopCloser(strings.NewReader(mockPackageHTML))
		defer file.Close()
		_versions, err := extractPythonPackageNames(file)
		if err != nil {
			return nil, fmt.Errorf("extract python package names: %w", err)
		}
		for _, v := range _versions {
			href := resolveHref(indexPath, v)
			split := strings.Split(href, "#")
			if len(split) > 1 {
				href = split[0]
			}
			hrefSplit := strings.Split(href, "/")
			version := ""
			if len(hrefSplit) > 0 {
				filename := hrefSplit[len(hrefSplit)-1]
				version = util.GetPyPIVersion(filename)
			}
			versions = append(versions, types.Version{
				Registry: registry,
				Pkg:      pkg,
				Path:     href,
				Name:     version,
				Size:     -1,
			})
		}
		return versions, nil
	}
	if artifactType == types.GO {
		var versions []types.Version
		if node == nil {
			return nil, errors.New("node is nil")
		}
		versionPath := p.Path + p.Name + "/@v"
		packageNode, err := tree.GetNodeForPath(node, versionPath)
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}
		leaves, err := tree.GetAllFiles(packageNode)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}
		versionMap := make(map[string]bool)
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}
			extension := filepath.Ext(leaf.Name)
			if extension != ".zip" {
				continue
			}
			versionName := strings.TrimSuffix(leaf.Name, extension)
			if _, ok := versionMap[versionName]; ok {
				continue
			}
			versionMap[versionName] = true
			versions = append(versions, types.Version{
				Registry: registry,
				Pkg:      pkg,
				Path:     versionPath,
				Name:     versionName,
				Size:     leaf.Size,
			})
		}
		return versions, nil
	}
	if artifactType == types.NPM {
		var versions []types.Version
		if node == nil {
			return nil, errors.New("node is nil")
		}

		// For NPM, we need to find all .tgz files for the specific package
		leaves, err := tree.GetAllFiles(node)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}

		versionMap := make(map[string]bool)
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}
			if !strings.Contains(leaf.Uri, ".tgz") {
				continue
			}

			// Extract package name and version from .tgz filename
			filename := leaf.Name
			if !strings.HasSuffix(filename, ".tgz") {
				continue
			}

			// Remove .tgz extension
			nameWithVersion := strings.TrimSuffix(filename, ".tgz")

			// Extract version from filename based on package type
			var version string
			if strings.HasPrefix(nameWithVersion, "@") {
				// For scoped packages like @angular-core-15.2.1
				parts := strings.Split(nameWithVersion, "-")
				if len(parts) >= 3 {
					// Check if this file belongs to the current package
					packagePart := strings.Join(parts[:len(parts)-1], "-")
					if packagePart == pkg {
						version = parts[len(parts)-1] // Last part is the version
					}
				}
			} else {
				// For regular packages like lodash-4.17.21
				lastHyphenIndex := strings.LastIndex(nameWithVersion, "-")
				if lastHyphenIndex > 0 {
					version = nameWithVersion[lastHyphenIndex+1:] // Everything after last hyphen is version
				}
			}

			// Skip if version is empty or already processed
			if version == "" || versionMap[version] {
				continue
			}

			versionMap[version] = true
			versions = append(versions, types.Version{
				Registry: registry,
				Pkg:      pkg,
				Path:     leaf.Uri,
				Name:     version,
				Size:     leaf.Size,
			})
		}
		return versions, nil
	}
	return []types.Version{}, errors.New("unknown artifact type")
}

func (a *adapter) GetFiles(registry string) ([]types.File, error) {
	files, err := a.client.getFiles(registry)
	if err != nil {
		log.Error().Msgf("Failed to get files from registry: %v", err)
		return nil, fmt.Errorf("failed to get files from registry: %w", err)
	}
	log.Info().Msgf("Get files from registry: %v", files)
	return files, nil
}

func (a *adapter) DownloadFile(registry string, uri string) (io.ReadCloser, http.Header, error) {
	return a.client.getFile(registry, uri)
}

func (a *adapter) GetOCIImagePath(registry string, image string) (string, error) {
	host, err := dockerHost(a.reg.Endpoint, registry)
	if err != nil {
		return "", fmt.Errorf("failed to get OCI host: %w", err)
	}
	return util.GenOCIImagePath(host, image), nil
}

func dockerHost(artifactoryBase, repo string) (string, error) {
	const suffix = ".jfrog.io"
	if !strings.HasSuffix(artifactoryBase, suffix) {
		return "", fmt.Errorf("not a jfrog.io host")
	}
	account := strings.TrimSuffix(artifactoryBase, suffix)
	endpoint := fmt.Sprintf("%s-%s%s", account, repo, suffix)
	parse, _ := url.Parse(endpoint)
	return parse.Host, nil
}

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
	return fmt.Errorf("not implemented")
}

func isMavenMetadataFile(filename string) bool {
	return filename == mavenMetadataFile ||
		filename == mavenMetadataFile+extensionMD5 ||
		filename == mavenMetadataFile+extensionSHA1 ||
		filename == mavenMetadataFile+extensionSHA256 ||
		filename == mavenMetadataFile+extensionSHA512
}

func (a *adapter) AddNPMTag(registry string, name string, version string, uri string) error {
	return nil
}

func (a *adapter) VersionExists(
	ctx context.Context,
	p types.Package,
	registryRef, pkg, version string,
	artifactType types.ArtifactType,
) (bool, error) {
	// Mock implementation - return true for common versions
	commonVersions := []string{"1.0.0", "1.1.0", "2.0.0", "latest"}
	for _, v := range commonVersions {
		if version == v {
			return true, nil
		}
	}
	return false, nil
}

func (a *adapter) FileExists(
	ctx context.Context,
	registry, pkg, version, fileName string,
	artifactType types.ArtifactType,
) (bool, error) {
	// Mock implementation - return true for common file types
	commonExtensions := []string{".jar", ".pom", ".tgz", ".tar.gz", ".zip", ".rpm", ".deb"}
	for _, ext := range commonExtensions {
		if strings.HasSuffix(fileName, ext) {
			return true, nil
		}
	}
	return false, nil
}

type repomdData struct {
	XMLName xml.Name `xml:"repomd"`
	Data    []struct {
		Type     string `xml:"type,attr"`
		Location struct {
			Href string `xml:"href,attr"`
		} `xml:"location"`
	} `xml:"data"`
}

type primaryPackage struct {
	XMLName  xml.Name `xml:"package"`
	Location struct {
		Href string `xml:"href,attr"`
	} `xml:"location"`
	Size struct {
		Package int `xml:"package,attr"`
	} `xml:"size"`
}

func (a *adapter) CreateVersion(
	registry string,
	artifactName string,
	version string,
	artifactType types.ArtifactType,
	files []*types.PackageFiles,
	metadata map[string]interface{},
) error {
	return nil
}

// ChartMetadata represents the metadata extracted from a Helm chart
type ChartMetadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
	AppVersion  string `yaml:"appVersion,omitempty"`
}

// HelmIndexEntry represents a minimal chart entry with only essential fields
type HelmIndexEntry struct {
	Name    string   `yaml:"name"`
	Version string   `yaml:"version"`
	URLs    []string `yaml:"urls"`
}

// HelmIndex represents a minimal Helm index structure
type HelmIndex struct {
	APIVersion string                      `yaml:"apiVersion"`
	Entries    map[string][]HelmIndexEntry `yaml:"entries"`
}

// GenerateHelmIndexFromPath dynamically generates Helm index.yaml content from a directory path containing .tgz files
func GenerateHelmIndexFromPath(chartsPath string) (string, error) {
	if chartsPath == "" {
		return "", fmt.Errorf("charts path cannot be empty")
	}

	// Check if the path exists
	if _, err := os.Stat(chartsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("charts path does not exist: %s", chartsPath)
	}

	// Find all .tgz files in the directory
	tgzFiles, err := filepath.Glob(filepath.Join(chartsPath, "*.tgz"))
	if err != nil {
		return "", fmt.Errorf("failed to find .tgz files: %w", err)
	}

	if len(tgzFiles) == 0 {
		return "", fmt.Errorf("no .tgz files found in directory: %s", chartsPath)
	}

	// Create the minimal index structure
	index := HelmIndex{
		APIVersion: "v1",
		Entries:    make(map[string][]HelmIndexEntry),
	}

	// Process each .tgz file
	for _, tgzFile := range tgzFiles {
		chartMetadata, err := extractChartMetadata(tgzFile)
		if err != nil {
			log.Warn().Msgf("Failed to extract metadata from %s: %v", tgzFile, err)
			continue
		}

		// Create minimal chart entry
		chartEntry := HelmIndexEntry{
			Name:    chartMetadata.Name,
			Version: chartMetadata.Version,
			URLs:    []string{filepath.Join(tgzFile)},
		}

		// Add to entries
		if index.Entries[chartMetadata.Name] == nil {
			index.Entries[chartMetadata.Name] = make([]HelmIndexEntry, 0)
		}
		index.Entries[chartMetadata.Name] = append(index.Entries[chartMetadata.Name], chartEntry)
	}

	// Convert to YAML
	yamlData, err := yaml.Marshal(&index)
	if err != nil {
		return "", fmt.Errorf("failed to marshal index to YAML: %w", err)
	}

	return string(yamlData), nil
}

// extractChartMetadata extracts metadata from a Helm chart .tgz file
func extractChartMetadata(tgzPath string) (*ChartMetadata, error) {
	file, err := os.Open(tgzPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .tgz file: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Look for Chart.yaml file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Check if this is the Chart.yaml file
		if strings.HasSuffix(header.Name, "/Chart.yaml") || header.Name == "Chart.yaml" {
			// Read the Chart.yaml content
			chartYamlContent, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read Chart.yaml content: %w", err)
			}

			// Parse the Chart.yaml
			var metadata ChartMetadata
			if err := yaml.Unmarshal(chartYamlContent, &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse Chart.yaml: %w", err)
			}

			return &metadata, nil
		}
	}

	return nil, fmt.Errorf("Chart.yaml not found in .tgz file")
}
