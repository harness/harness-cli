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
	client Client
	reg    types.RegistryConfig
}

func newAdapter(config types.RegistryConfig) (adp.Adapter, error) {
	return &adapter{
		client: newClient(&config),
		reg:    config,
	}, nil
}

// NewAdapterWithClient creates an adapter with a custom Client implementation.
// This is used by mock_jfrog to inject a mock client while reusing all adapter logic.
func NewAdapterWithClient(config types.RegistryConfig, c Client) adp.Adapter {
	return &adapter{client: c, reg: config}
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
	reg, err := a.client.GetRegistry(registry)
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
	if artifactType == types.GENERIC || artifactType == types.RAW {
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "default",
			Size:     -1,
		})
	} else if artifactType == types.MAVEN || artifactType == types.NPM {
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     "/",
			Name:     "",
			Size:     -1,
		})
	} else if artifactType == types.NUGET {
		// Extract unique package names from NUGET files in the tree
		files, err := tree.GetAllFiles(root)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}

		pkgMap := make(map[string]bool)
		for _, file := range files {
			if file.Folder {
				continue
			}

			pkgName, _, ok := util.ParseNugetFileNameWithPath(file.Uri)

			if !ok {
				continue
			}
			pkgMap[pkgName] = true
		}

		for pkgName := range pkgMap {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     pkgName,
				Size:     -1,
			})
		}
		log.Info().Msgf("Found %d NUGET packages", len(packages))
	} else if artifactType == types.DOCKER || artifactType == types.HELM {
		catalog, err := a.client.GetCatalog(registry)
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
		indexPkgs, err := a.enumerateHelmIndex(registry, root)
		if err != nil {
			return nil, err
		}
		packages = append(packages, indexPkgs...)
	} else if artifactType == types.HELM_HTTP {
		// Hybrid enumeration, tree-primary.
		//
		// The tree sweep is the source of truth for physical layout: every .tgz
		// on disk yields its FULL nested package name (directory prefix + leaf)
		// plus version. This keeps genuinely-distinct nested charts distinct —
		// "team-a/abc-1.0.1.tgz" and "team-b/abc-1.0.1.tgz" are the separate
		// identities "team-a/abc" and "team-b/abc" (same version), and both must
		// survive (each is its own package_name in HAR storage).
		//
		// The index.yaml is the secondary source: it only contributes charts
		// that are listed but NOT present on disk (a stale index). Dedup between
		// the two is keyed on the chart's repository-relative path, so a chart
		// appearing in both sources is enumerated once — under the tree's nested
		// name, which is more reliable than the index's: JFrog defaults to
		// relative index URLs (since 7.59.5), for which getNestedName degrades to
		// the bare leaf name. Keying dedup on the path (not name+version) avoids
		// both collisions (distinct nested charts surviving) and the relative-URL
		// name divergence (same file matched across sources).
		seenPath := make(map[string]bool) // repo-relative .tgz path

		// 1. Tree sweep (ground truth for physical charts + full nested names).
		files, err := tree.GetAllFiles(root)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}
		for _, f := range files {
			if f.Folder {
				continue
			}
			if !util.IsHelmChartArchive(f.Uri) {
				continue
			}
			leafName, ver, ok := util.ParseChartFileName(f.Name)
			if !ok {
				log.Warn().Msgf(
					"HELM_HTTP: skipping chart file with non-conforming name (cannot parse <name>-<version>): %s", f.Uri)
				continue
			}
			relPath := strings.TrimPrefix(f.Uri, "/")
			seenPath[relPath] = true

			// Preserve the nested directory prefix so the upload mirrors the
			// source layout (e.g. "team-a/abc"); flat charts keep the bare leaf.
			nestedName := leafName
			if dir := strings.Trim(path.Dir(relPath), "/"); dir != "" && dir != "." {
				nestedName = dir + "/" + leafName
			}
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     nestedName,
				Version:  ver,
				Size:     f.Size,
				URL:      f.Uri,
			})
		}

		// 2. Index pass (recovers charts in index.yaml but missing on disk). A
		//    missing/unreadable index is not fatal — the tree sweep already
		//    covers physical charts — so we warn and continue.
		indexPkgs, err := a.enumerateHelmIndex(registry, root)
		if err != nil {
			log.Warn().Err(err).Msgf(
				"HELM_HTTP: failed to enumerate index.yaml for registry %s; relying on tree sweep only", registry)
		}
		for _, pkg := range indexPkgs {
			relPath := chartRepoRelPath(pkg.URL)
			if seenPath[relPath] {
				// Same physical chart already enumerated from the tree (with its
				// full nested name) — skip the index's representation.
				continue
			}
			seenPath[relPath] = true
			packages = append(packages, pkg)
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
	} else if artifactType == types.DEBIAN {
		// Get the dists node
		distsNode, err := tree.GetNodeForPath(root, "/dists")
		if err != nil {
			return nil, fmt.Errorf("get dists folder: %w", err)
		}

		// Iterate through distributions (subdirectories of dists)
		for _, distNode := range distsNode.Children {
			if !distNode.IsLeaf {
				// Check for InRelease or Release file to validate distribution
				// Try InRelease first (GPG-signed inline version)
				releaseNode, err := tree.GetNodeForPath(&distNode, "InRelease")
				if err != nil {
					// Fall back to Release file
					releaseNode, err = tree.GetNodeForPath(&distNode, "Release")
					if err != nil {
						log.Warn().Msgf("Skipping %s: no Release or InRelease file found", distNode.Name)
						continue
					}
				}

				// Download and parse Release file to get components
				releaseFile, _, err := a.DownloadFile(registry, releaseNode.File.Uri)
				if err != nil {
					log.Warn().Msgf("Failed to download Release file for %s: %v", distNode.Name, err)
					continue
				}

				components, architectures, err := parseDebianRelease(releaseFile)
				releaseFile.Close()
				if err != nil {
					log.Warn().Msgf("Failed to parse Release file for %s: %v", distNode.Name, err)
					continue
				}

				// Iterate through components and architectures
				for _, component := range components {
					// Process binary packages for each architecture
					for _, arch := range architectures {
						// Path to Packages file
						packagesPath := fmt.Sprintf("/dists/%s/%s/binary-%s/Packages", distNode.Name, component, arch)
						packagesNode, err := tree.GetNodeForPath(root, packagesPath)
						if err != nil {
							// Try .gz version
							packagesPath = fmt.Sprintf("/dists/%s/%s/binary-%s/Packages.gz", distNode.Name, component, arch)
							packagesNode, err = tree.GetNodeForPath(root, packagesPath)
							if err != nil {
								log.Warn().Msgf("Packages file not found: %s", packagesPath)
								continue
							}
						}

						// Download and parse Packages file
						packagesFile, _, err := a.DownloadFile(registry, packagesNode.File.Uri)
						if err != nil {
							log.Warn().Msgf("Failed to download Packages file %s: %v", packagesPath, err)
							continue
						}

						debPackages, err := extractDebianPackages(packagesFile, registry, distNode.Name, component, strings.HasSuffix(packagesPath, ".gz"))
						packagesFile.Close()
						if err != nil {
							log.Warn().Msgf("Failed to extract Debian packages from %s: %v", packagesPath, err)
							continue
						}

						packages = append(packages, debPackages...)
					}

					// Process source packages for this component
					sourcesPath := fmt.Sprintf("/dists/%s/%s/source/Sources", distNode.Name, component)
					sourcesNode, err := tree.GetNodeForPath(root, sourcesPath)
					if err != nil {
						// Try .gz version
						sourcesPath = fmt.Sprintf("/dists/%s/%s/source/Sources.gz", distNode.Name, component)
						sourcesNode, err = tree.GetNodeForPath(root, sourcesPath)
						if err != nil {
							log.Debug().Msgf("Sources file not found: %s (this is normal if no source packages)", sourcesPath)
							continue
						}
					}

					// Download and parse Sources file
					sourcesFile, _, err := a.DownloadFile(registry, sourcesNode.File.Uri)
					if err != nil {
						log.Warn().Msgf("Failed to download Sources file %s: %v", sourcesPath, err)
						continue
					}

					sourcePackages, err := extractDebianSourcePackages(sourcesFile, registry, distNode.Name, component, strings.HasSuffix(sourcesPath, ".gz"))
					sourcesFile.Close()
					if err != nil {
						log.Warn().Msgf("Failed to extract Debian source packages from %s: %v", sourcesPath, err)
						continue
					}

					packages = append(packages, sourcePackages...)
				}
			}
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
	} else if artifactType == types.SWIFT {
		leaves, _ := tree.GetAllFiles(root)
		packageMap := make(map[string]bool)
		for _, leaf := range leaves {
			if leaf.Folder {
				continue
			}
			if !strings.HasSuffix(leaf.Uri, ".zip") {
				continue
			}

			// Swift packages in JFrog have two possible URI formats:
			// 4-segment: /<scope>/<name>/<version>/<name>-<version>.zip (Publish API)
			// 3-segment: /<scope>/<name>/<name>-<version>.zip (direct deploy)
			uriPath := strings.TrimPrefix(leaf.Uri, "/")
			parts := strings.Split(uriPath, "/")

			var scope, name, version string
			if len(parts) >= 4 {
				scope = parts[0]
				name = parts[1]
				version = parts[2]
			} else if len(parts) == 3 {
				scope = parts[0]
				name = parts[1]
				// Extract version from filename: <name>-<version>.zip
				filename := strings.TrimSuffix(parts[2], ".zip")
				if strings.HasPrefix(filename, name+"-") {
					version = strings.TrimPrefix(filename, name+"-")
				} else {
					continue
				}
			} else {
				continue
			}

			// Package name in scope.name format
			pkgName := scope + "." + name
			pkgKey := pkgName + "-" + version

			path := "/"
			if _, ok := packageMap[pkgKey]; ok {
				continue
			}
			packageMap[pkgKey] = true
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     path,
				Name:     pkgName,
				Version:  version,
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
	} else if artifactType == types.PUPPET {
		// Puppet modules in JFrog follow the Forge layout
		// <repo>/<author>/<module>/<author>-<module>-<version>.tar.gz, but
		// we don't depend on the directory structure: any .tar.gz whose
		// filename matches "<author>-<module>-<version>" is a candidate.
		files, err := tree.GetAllFiles(root)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}

		pkgMap := make(map[string]bool)
		for _, file := range files {
			if file.Folder {
				continue
			}
			pkgName, _, ok := util.ParsePuppetFileNameWithPath(file.Uri)
			if !ok {
				continue
			}
			pkgMap[pkgName] = true
		}

		for pkgName := range pkgMap {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     pkgName,
				Size:     -1,
			})
		}
		log.Info().Msgf("Found %d PUPPET packages", len(packages))
	} else if artifactType == types.CONAN {
		// One package per distinct Conan reference (name/version[@user/channel]).
		// The reference subtree carries every RREV/PKGID/PREV file, migrated by
		// migrateConan.
		files, err := tree.GetAllFiles(root)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}
		packages = append(packages, util.GetConanPackages(files, registry)...)
		log.Info().Msgf("Found %d CONAN packages", len(packages))
	} else {
		return []types.Package{}, errors.New("unknown artifact type")
	}

	return packages, nil
}

// enumerateHelmIndex parses the repository's /index.yaml and returns one
// package per chart entry (name+version). Chart names are derived via
// getNestedName so the nested directory prefix is preserved (when the index URL
// is absolute). Shared by the HELM_LEGACY and HELM_HTTP enumeration paths.
func (a *adapter) enumerateHelmIndex(registry string, root *types.TreeNode) ([]types.Package, error) {
	node, err := tree.GetNodeForPath(root, "/index.yaml")
	if err != nil {
		return nil, fmt.Errorf("get node for path: %w", err)
	}
	file, _, err := a.DownloadFile(registry, node.File.Uri)
	if err != nil {
		return nil, fmt.Errorf("download index.yaml: %w", err)
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "index-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create temp index file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		return nil, fmt.Errorf("copy index.yaml to temp: %w", err)
	}
	index, err := repo.LoadIndexFile(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("load index file: %w", err)
	}

	var packages []types.Package
	for name, entries := range index.Entries {
		for _, ver := range entries {
			if len(ver.URLs) == 0 {
				log.Warn().Msgf(
					"Skipping helm chart with no URLs for registry: %s, name: %s, version: %s",
					registry, name, ver.Version)
				continue
			}
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

			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     nestedName,
				Size:     -1,
				URL:      chartUrl,
				Version:  ver.Version,
			})
		}
	}
	return packages, nil
}

// chartRepoRelPath normalizes a Helm index chart URL to the repository-relative
// path (e.g. "team-a/abc-1.0.1.tgz"), matching the form the tree sweep derives
// from a file's repo-relative Uri. This lets HELM_HTTP enumeration dedup the
// same physical chart across the index and the tree regardless of whether the
// index URL is absolute (/artifactory/<repo>/<dirs>/<file>, the pre-7.59.5
// default) or relative (<dirs>/<file>, the current default). The absolute case
// drops the leading "", "artifactory", and "<repo>" segments — the same
// splits[3:] convention getNestedName uses.
func chartRepoRelPath(chartURL string) string {
	parsed, err := url.Parse(chartURL)
	if err != nil {
		return strings.TrimPrefix(chartURL, "/")
	}
	p := parsed.Path
	if strings.HasPrefix(p, "/") {
		splits := strings.Split(p, "/")
		if len(splits) >= 4 {
			return strings.Join(splits[3:], "/")
		}
		return strings.TrimPrefix(p, "/")
	}
	return p
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

func parseDebianRelease(file io.Reader) ([]string, []string, error) {
	var components []string
	var architectures []string

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("read Release file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Components:") {
			componentStr := strings.TrimPrefix(line, "Components:")
			components = strings.Fields(strings.TrimSpace(componentStr))
		} else if strings.HasPrefix(line, "Architectures:") {
			archStr := strings.TrimPrefix(line, "Architectures:")
			architectures = strings.Fields(strings.TrimSpace(archStr))
		}
	}

	if len(components) == 0 {
		return nil, nil, fmt.Errorf("no components found in Release file")
	}
	if len(architectures) == 0 {
		return nil, nil, fmt.Errorf("no architectures found in Release file")
	}

	return components, architectures, nil
}

func extractDebianPackages(file io.Reader, registry string, distribution string, component string, isGzipped bool) ([]types.Package, error) {
	var reader io.Reader = file

	if isGzipped {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read Packages file: %w", err)
	}

	var packages []types.Package
	paragraphs := strings.Split(string(data), "\n\n")

	for _, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			continue
		}

		var filename string
		var size int

		lines := strings.Split(paragraph, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Filename:") {
				filename = strings.TrimSpace(strings.TrimPrefix(line, "Filename:"))
			} else if strings.HasPrefix(line, "Size:") {
				sizeStr := strings.TrimSpace(strings.TrimPrefix(line, "Size:"))
				fmt.Sscanf(sizeStr, "%d", &size)
			}
		}

		if filename != "" {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     path.Base(filename),
				URL:      filename,
				Size:     size,
				Metadata: map[string]string{
					"distribution": distribution,
					"component":    component,
				},
			})
		}
	}

	return packages, nil
}

func extractDebianSourcePackages(file io.Reader, registry string, distribution string, component string, isGzipped bool) ([]types.Package, error) {
	var reader = file

	if isGzipped {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read Sources file: %w", err)
	}

	var packages []types.Package
	paragraphs := strings.Split(string(data), "\n\n")

	for _, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			continue
		}

		var packageName string
		var version string
		var directory string
		var dscFiles []types.Package
		var allFiles []string // Track all filenames for metadata

		lines := strings.Split(paragraph, "\n")
		inFilesSection := false

		for _, line := range lines {
			// Extract Package name
			if strings.HasPrefix(line, "Package:") {
				packageName = strings.TrimSpace(strings.TrimPrefix(line, "Package:"))
				inFilesSection = false
				continue
			}

			// Extract Version
			if strings.HasPrefix(line, "Version:") {
				version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
				inFilesSection = false
				continue
			}

			// Check if we're in the Files section
			if strings.HasPrefix(line, "Files:") {
				inFilesSection = true
				continue
			}

			// Check if we're in the Directory field
			if strings.HasPrefix(line, "Directory:") {
				directory = strings.TrimSpace(strings.TrimPrefix(line, "Directory:"))
				inFilesSection = false
				continue
			}

			// If line doesn't start with space, we're out of Files section
			if inFilesSection && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inFilesSection = false
			}

			// Parse file entries in Files section
			if inFilesSection && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
				// Files format: <md5sum> <size> <filename>
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					filename := fields[2]

					// Track all filenames for metadata
					allFiles = append(allFiles, filename)

					// Only create package entries for .dsc files
					// Source files will be uploaded when the .dsc is processed
					if strings.HasSuffix(filename, ".dsc") {
						size := 0
						fmt.Sscanf(fields[1], "%d", &size)

						var filePath string
						if directory != "" {
							filePath = directory + "/" + filename
						} else {
							filePath = filename
						}

						pkg := types.Package{
							Registry: registry,
							Path:     "/",
							Name:     filename,
							URL:      filePath,
							Size:     size,
							Metadata: map[string]string{
								"distribution": distribution,
								"component":    component,
								"packageName":  packageName,
								"fullVersion":  version,
							},
						}

						dscFiles = append(dscFiles, pkg)
					}
				}
			}
		}

		// Add source file information to .dsc file metadata
		for i := range dscFiles {
			var sourceFiles []string
			for _, filename := range allFiles {
				if strings.Contains(filename, ".orig.tar.") || strings.Contains(filename, ".tar.") {
					sourceFiles = append(sourceFiles, filename)
				}
			}
			if len(sourceFiles) > 0 {
				dscFiles[i].Metadata["sourceFiles"] = strings.Join(sourceFiles, ",")
			}
			// Store directory path for locating source files
			if directory != "" {
				dscFiles[i].Metadata["directory"] = directory
			}
		}

		// Only add .dsc files as packages
		// Source files (otherFiles) will be automatically uploaded when the .dsc file is processed
		// by the migrateDebian function in migratable/package.go
		packages = append(packages, dscFiles...)
	}

	return packages, nil
}

func (a *adapter) GetVersions(
	p types.Package,
	node *types.TreeNode,
	registry, pkg string,
	artifactType types.ArtifactType,
) ([]types.Version, error) {
	if artifactType == types.GENERIC || artifactType == types.RAW {
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

	if artifactType == types.MAVEN || artifactType == types.NPM {
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

	if artifactType == types.NUGET {
		// Extract versions for this NUGET package from the file tree
		files, err := tree.GetAllFiles(node)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}

		versionMap := make(map[string]bool)
		for _, file := range files {
			if file.Folder {
				continue
			}

			pkgName, version, ok := util.ParseNugetFileNameWithPath(file.Uri)
			if !ok || pkgName != pkg {
				continue
			}
			versionMap[version] = true
		}

		var versions []types.Version
		for version := range versionMap {
			versions = append(versions, types.Version{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     version,
				Size:     -1,
			})
		}
		log.Info().Msgf("Found %d versions for NUGET package %s", len(versions), pkg)
		return versions, nil
	}

	if artifactType == types.HELM_LEGACY || artifactType == types.HELM_HTTP {
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
		file, _, err := a.client.GetFile(registry, indexPath)
		if err != nil {
			// Fall back to extracting versions from the file tree when .pypi
			// metadata files don't exist (e.g. packages deployed directly via
			// Jenkins or the generic deploy API, not via the PyPI upload API).
			log.Warn().Err(err).Msgf(
				"Failed to get .pypi index for %s, falling back to file tree", pkg)
			return a.getPythonVersionsFromTree(node, registry, pkg)
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
	if artifactType == types.SWIFT {
		var versions []types.Version
		versions = append(versions, types.Version{
			Registry: registry,
			Pkg:      pkg,
			Path:     p.URL,
			Name:     p.Version,
			Size:     p.Size,
		})
		return versions, nil
	}
	if artifactType == types.PUPPET {
		files, err := tree.GetAllFiles(node)
		if err != nil {
			return nil, fmt.Errorf("get all files: %w", err)
		}

		versionMap := make(map[string]bool)
		for _, file := range files {
			if file.Folder {
				continue
			}
			pkgName, version, ok := util.ParsePuppetFileNameWithPath(file.Uri)
			if !ok || pkgName != pkg {
				continue
			}
			versionMap[version] = true
		}

		var versions []types.Version
		for version := range versionMap {
			versions = append(versions, types.Version{
				Registry: registry,
				Pkg:      pkg,
				Path:     "/",
				Name:     version,
				Size:     -1,
			})
		}
		log.Info().Msgf("Found %d versions for PUPPET package %s", len(versions), pkg)
		return versions, nil
	}
	return []types.Version{}, errors.New("unknown artifact type")
}

func (a *adapter) GetFiles(registry string) ([]types.File, error) {
	files, err := a.client.GetFiles(registry)
	if err != nil {
		log.Error().Msgf("Failed to get files from registry: %v", err)
		return nil, fmt.Errorf("failed to get files from registry: %w", err)
	}
	log.Info().Msgf("Get files from registry: %v", files)
	return files, nil
}

func (a *adapter) DownloadFile(registry string, uri string) (io.ReadCloser, http.Header, error) {
	return a.client.GetFile(registry, uri)
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

// getPythonVersionsFromTree extracts Python package versions by scanning the
// file tree. This is used as a fallback when the .pypi index HTML files are
// not available (e.g. packages deployed directly, not via the PyPI API).
func (a *adapter) getPythonVersionsFromTree(
	node *types.TreeNode,
	registry, pkg string,
) ([]types.Version, error) {
	if node == nil {
		return nil, errors.New("node is nil")
	}

	pkgNode, err := tree.GetNodeForPath(node, pkg)
	if err != nil {
		return nil, fmt.Errorf("package path %q not found in file tree: %w", pkg, err)
	}

	files, err := tree.GetAllFiles(pkgNode)
	if err != nil {
		return nil, fmt.Errorf("get files for package %s: %w", pkg, err)
	}

	var versions []types.Version
	seen := make(map[string]bool)
	for _, f := range files {
		if f.Folder {
			continue
		}
		version := util.GetPyPIVersion(f.Name)
		if version == "" {
			continue
		}
		if seen[version] {
			continue
		}
		seen[version] = true

		// Use the directory containing this file as the version path
		versionPath := path.Dir(f.Uri)
		versions = append(versions, types.Version{
			Registry: registry,
			Pkg:      pkg,
			Path:     versionPath,
			Name:     version,
			Size:     -1,
		})
	}

	log.Info().Msgf("Found %d versions for package %s from file tree", len(versions), pkg)
	return versions, nil
}
