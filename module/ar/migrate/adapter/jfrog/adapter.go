package jfrog

import (
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
	adapterType := types.JFROG
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

func (a *adapter) GetKeyChain(sourcePackageHostname string) (authn.Keychain, error) {
	var host string
	if sourcePackageHostname != "" {
		host = sourcePackageHostname
	} else {
		parse, err := url.Parse(a.reg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to parse [%s], err: %w", a.reg.Endpoint, err)
		}
		host = parse.Host
	}
	return NewJfrogKeychain(a.reg.Credentials.Username, a.reg.Credentials.Password, host), nil
}

func (a *adapter) GetConfig() types.RegistryConfig {
	return a.reg
}

func (a *adapter) ValidateCredentials() (bool, error) { return false, nil }
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
func (a *adapter) CreateRegistryIfDoesntExist(registry string) (bool, error) { return false, nil }

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

		log.Info().Msgf("OCI catalog: %v", catalog)
		for _, repo := range catalog {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     repo,
				Size:     -1,
			})
		}
	} else if artifactType == types.HELM_LEGACY {
		node, err := tree.GetNodeForPath(root, "/index.yaml")
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}
		file, _, err := a.DownloadFile(registry, node.File.Uri)
		tmp, err := os.CreateTemp("", "index-*.yaml")
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, file)
		index, err := repo.LoadIndexFile(tmp.Name())
		if err != nil {
			return nil, fmt.Errorf("load index file: %w", err)
		}

		for name, entries := range index.Entries {
			for _, ver := range entries {
				nestedName, err2 := getNestedName(name, ver.URLs)
				if err2 != nil {
					log.Error().Err(err2).Msgf("Failed to get package name for registry: %s, name: %s, version: %s",
						registry, name, ver.Version)
					continue
				}
				chartUrl := ver.URLs[0]
				if strings.HasPrefix(chartUrl, "local://") {
					chartUrl = strings.TrimPrefix(chartUrl, "local://")
				}

				pkg := types.Package{
					Registry: registry,
					Path:     "/",
					Name:     nestedName,
					Size:     -1,
					URL:      chartUrl,
					Version:  ver.Version,
				}
				packages = append(packages, pkg)
			}
		}
	} else if artifactType == types.PYTHON {

		node, err := tree.GetNodeForPath(root, "/.pypi/simple.html")
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}
		file, _, err := a.DownloadFile(registry, node.File.Uri)
		if err != nil {
			return nil, fmt.Errorf("download file: %w", err)
		}
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
		node, err := tree.GetNodeForPath(root, "/repodata/repomd.xml")
		if err != nil {
			return nil, fmt.Errorf("get node for path: %w", err)
		}
		file, _, err := a.DownloadFile(registry, node.File.Uri)
		if err != nil {
			return nil, fmt.Errorf("download repomd.xml: %w", err)
		}
		defer file.Close()

		primaryLocation, err := extractPrimaryLocation(file)
		if err != nil {
			return nil, fmt.Errorf("extract primary location: %w", err)
		}

		primaryFile, _, err := a.DownloadFile(registry, primaryLocation)
		if err != nil {
			return nil, fmt.Errorf("download primary file: %w", err)
		}
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
	} else if artifactType == types.CONDA {
		packages, err := GetCondaPackagesFromTreeNode(a, root, registry)
		if err != nil {
			return nil, fmt.Errorf("get packages from tree node: %w", err)
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
	} else if artifactType == types.COMPOSER {
		leaves, _ := tree.GetAllFiles(root)
		packageMap := make(map[string]bool)
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}
			if !strings.HasSuffix(leaf.Uri, ".zip") {
				continue
			}
			// Extract package name from ZIP filename
			// Composer packages are typically named: vendor-package-version.zip
			filename := leaf.Name
			nameWithoutExt := strings.TrimSuffix(filename, ".zip")

			// For Composer, we'll use the full filename as package name
			// since Composer packages can have complex naming patterns
			pkgName := nameWithoutExt

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
				URL:      leaf.Uri,
			})
		}
		return packages, nil
	} else if artifactType == types.DART {
		// Treat Dart like a generic bucket: one logical package
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "default",
			Size:     -1,
		})
		return packages, nil
	} else {
		return []types.Package{}, errors.New("unknown artifact type")
	}

	return packages, nil
}

func getNestedName(packageName string, urls []string) (string, error) {
	if urls == nil || len(urls) == 0 {
		return "", fmt.Errorf("url is invalid")
	}

	parsedURL, err := url.Parse(urls[0])
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	splits := strings.Split(parsedURL.Path, "/")
	if len(splits) < 5 {
		return packageName, nil
	}
	return strings.Join(splits[3:len(splits)-1], "/") + "/" + packageName, nil
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
		file, _, err := a.client.getFile(registry, indexPath)
		if err != nil {
			return nil, fmt.Errorf("download file: %w", err)
		}
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
	if artifactType == types.DART {
		var versions []types.Version
		if node == nil {
			return nil, errors.New("node is nil")
		}

		// For Dart, find all .tar.gz files and extract version from filename
		// Dart package filename format: <package_name>-<version>.tar.gz
		leaves, err := tree.GetAllFiles(node)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}

			filename := leaf.Name
			if !strings.HasSuffix(filename, ".tar.gz") {
				continue
			}

			// Remove .tar.gz extension
			nameWithVersion := strings.TrimSuffix(filename, ".tar.gz")

			versions = append(versions, types.Version{
				Registry: registry,
				Pkg:      pkg,
				Path:     leaf.Uri,
				Name:     nameWithVersion,
				Size:     leaf.Size,
			})
		}
		return versions, nil
	}
	if artifactType == types.COMPOSER {
		var versions []types.Version
		versions = append(versions, types.Version{
			Registry: registry,
			Pkg:      pkg,
			Path:     p.URL,
			Name:     p.Name,
			Size:     p.Size,
		})
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

func (a *adapter) GetOCIImagePath(registry string, packageHostname string, image string) (string, error) {
	var host string
	if packageHostname != "" {
		host = packageHostname
	} else {
		parse, err := url.Parse(a.reg.Endpoint)
		if err != nil {
			return "", fmt.Errorf("failed to get OCI host: %w", err)
		}
		host = parse.Host
	}
	return util.GenOCIImagePath(host, registry, image), nil
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
	return false, fmt.Errorf("not implemented")
}

func (a *adapter) FileExists(
	ctx context.Context,
	registryRef, pkg, version string,
	fileName *types.File,
	artifactType types.ArtifactType,
) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (a *adapter) GetAllFilesForVersion(
	ctx context.Context,
	registryRef, pkg, version string,
) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
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
