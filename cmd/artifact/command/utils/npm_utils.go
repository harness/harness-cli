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
	PeerDependenciesMeta interface{}       `json:"peerDependenciesMeta"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	AcceptDependencies   interface{}       `json:"acceptDependencies"`
	BundleDependencies   interface{}       `json:"bundleDependencies"`
	Bin                  interface{}       `json:"bin"`
	Contributors         interface{}       `json:"contributors"`
	Bugs                 interface{}       `json:"bugs"`
	Engines              interface{}       `json:"engines"`
	Deprecated           interface{}       `json:"deprecated"`
	Directories          interface{}       `json:"directories"`
	Funding              interface{}       `json:"funding"`
	CPU                  interface{}       `json:"cpu"`
	OS                   interface{}       `json:"os"`
	Main                 interface{}       `json:"main"`
	Module               interface{}       `json:"module"`
	Types                interface{}       `json:"types"`
	Typings              interface{}       `json:"typings"`
	Exports              interface{}       `json:"exports"`
	Imports              interface{}       `json:"imports"`
	Files                interface{}       `json:"files"`
	Workspaces           interface{}       `json:"workspaces"`
	Scripts              interface{}       `json:"scripts"`
	Config               interface{}       `json:"config"`
	PublishConfig        interface{}       `json:"publishConfig"`
	SideEffects          interface{}       `json:"sideEffects"`
	HasShrinkwrap        interface{}       `json:"_hasShrinkwrap"`
	HasInstallScript     interface{}       `json:"hasInstallScript"`
	NodeVersion          interface{}       `json:"_nodeVersion"`
	NpmUser              interface{}       `json:"_npmUser"`
	NpmVersion           interface{}       `json:"_npmVersion"`
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
		BundleDependencies:   pkg.BundleDependencies,
		DevDependencies:      pkg.DevDependencies,
		PeerDependencies:     pkg.PeerDependencies,
		PeerDependenciesMeta: pkg.PeerDependenciesMeta,
		Bin:                  pkg.Bin,
		OptionalDependencies: pkg.OptionalDependencies,
		AcceptDependencies:   pkg.AcceptDependencies,
		Readme:               "",
		Dist:                 npm.PackageDistribution{},
		Maintainers:          nil,
		Contributors:         pkg.Contributors,
		Bugs:                 pkg.Bugs,
		Engines:              pkg.Engines,
		Deprecated:           pkg.Deprecated,
		Directories:          pkg.Directories,
		Funding:              pkg.Funding,
		CPU:                  pkg.CPU,
		OS:                   pkg.OS,
		Main:                 pkg.Main,
		Module:               pkg.Module,
		Types:                pkg.Types,
		Typings:              pkg.Typings,
		Exports:              pkg.Exports,
		Imports:              pkg.Imports,
		Files:                pkg.Files,
		Workspaces:           pkg.Workspaces,
		Scripts:              pkg.Scripts,
		Config:               pkg.Config,
		PublishConfig:        pkg.PublishConfig,
		SideEffects:          pkg.SideEffects,
		HasShrinkwrap:        pkg.HasShrinkwrap,
		HasInstallScript:     pkg.HasInstallScript,
		NodeVersion:          pkg.NodeVersion,
		NpmUser:              pkg.NpmUser,
		NpmVersion:           pkg.NpmVersion,
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
		Contributors:   pkg.Contributors,
		Time:           nil,
		Homepage:       pkg.Homepage,
		Keywords:       pkg.Keywords,
		Repository:     pkg.Repository,
		Author:         pkg.Author,
		Bugs:           pkg.Bugs,
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
