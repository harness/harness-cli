package utils

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/harness/harness-cli/module/ar/migrate/types/npm"
)

func ExtractPackageJSONFromTarball(file io.ReadCloser) ([]byte, error) {
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.FileInfo().IsDir() {
			continue
		}

		base := filepath.Base(header.Name)
		if base == "package.json" {
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read package.json from tarball: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("package.json not found in tarball")
}

// minimalPackageJSON represents the subset of fields from package.json we care about.
type MinimalPackageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	Homepage             string            `json:"homepage"`
	Keywords             []string          `json:"keywords"`
	Repository           interface{}       `json:"repository"`
	Author               interface{}       `json:"author"`
	License              interface{}       `json:"license"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Bin                  interface{}       `json:"bin"`
}

func BuildNpmUploadFromPackageJSON(pkgJSON []byte, file io.ReadCloser) (*npm.PackageUpload, string, string, error) {
	var pkg MinimalPackageJSON
	if err := json.Unmarshal(pkgJSON, &pkg); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse package.json: %w", err)
	}

	if pkg.Name == "" || pkg.Version == "" {
		return nil, "", "", fmt.Errorf("package.json must contain 'name' and 'version'")
	}

	versionObj := &npm.PackageMetadataVersion{
		ID:                   pkg.Name + "@" + pkg.Version,
		Name:                 pkg.Name,
		Version:              pkg.Version,
		Description:          pkg.Description,
		Author:               pkg.Author,
		Homepage:             pkg.Homepage,
		License:              pkg.License,
		Repository:           pkg.Repository,
		Keywords:             pkg.Keywords,
		Dependencies:         pkg.Dependencies,
		BundleDependencies:   nil,
		DevDependencies:      pkg.DevDependencies,
		PeerDependencies:     pkg.PeerDependencies,
		Bin:                  pkg.Bin,
		OptionalDependencies: pkg.OptionalDependencies,
		Readme:               "",
		Dist:                 npm.PackageDistribution{},
		Maintainers:          nil,
	}

	metadata := npm.PackageMetadata{
		ID:          pkg.Name,
		Name:        pkg.Name,
		Description: pkg.Description,
		DistTags: map[string]string{
			"latest": pkg.Version,
		},
		Versions: map[string]*npm.PackageMetadataVersion{
			pkg.Version: versionObj,
		},
		Readme:         "",
		Maintainers:    nil,
		Time:           nil,
		Homepage:       pkg.Homepage,
		Keywords:       pkg.Keywords,
		Repository:     pkg.Repository,
		Author:         pkg.Author,
		ReadmeFilename: "",
		Users:          nil,
		License:        pkg.License,
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read tarball for attachment: %w", err)
	}

	b64Data := base64.StdEncoding.EncodeToString(data)

	// generate tarball name from package name and version
	tarballName := pkg.Name + "-" + pkg.Version + ".tgz"

	upload := &npm.PackageUpload{
		PackageMetadata: metadata,
		Attachments: map[string]*npm.PackageAttachment{
			tarballName: {
				ContentType: "application/octet-stream",
				Data:        b64Data,
				Length:      len(data),
			},
		},
	}

	return upload, pkg.Name, pkg.Version, nil
}
