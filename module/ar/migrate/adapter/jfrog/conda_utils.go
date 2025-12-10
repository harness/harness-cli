package jfrog

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog/log"
)

func GetCondaPackagesFromTreeNode(
	a *adapter,
	root *types.TreeNode,
	registry string,
) ([]types.Package, error) {
	leaves, _ := tree.GetAllFiles(root)
	var packages []types.Package
	// Build a map of directories to their repodata.json paths
	repodataMap := make(map[string]string)
	for _, leaf := range leaves {
		if leaf.Folder {
			continue
		}
		if strings.HasSuffix(leaf.Uri, "/repodata.json") {
			dir := path.Dir(leaf.Uri)
			repodataMap[dir] = leaf.Uri
		}
	}

	// Define conda package structure
	type condaPackage struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Size int    `json:"size"`
	}

	// Create map[repodata_path][]condaPackage
	condaPackagesByRepodata := make(map[string][]condaPackage)

	for _, leaf := range leaves {
		if leaf.Folder {
			continue
		}

		// Check if file is .conda or .tar.bz2
		if strings.HasSuffix(leaf.Uri, ".conda") || strings.HasSuffix(leaf.Uri, ".tar.bz2") {
			dir := path.Dir(leaf.Uri)

			// Find repodata.json in the same directory
			if repodataPath, exists := repodataMap[dir]; exists {
				pkg := condaPackage{
					Name: leaf.Name,
					Path: leaf.Uri,
					Size: leaf.Size,
				}
				condaPackagesByRepodata[repodataPath] = append(condaPackagesByRepodata[repodataPath], pkg)
			}
		}
	}

	// loop through each repodata path, download repodata and extract subdir
	for repodataPath, pkgs := range condaPackagesByRepodata {
		file, _, err := a.DownloadFile(registry, repodataPath)
		if err != nil {
			return nil, fmt.Errorf("download repodata: %w", err)
		}
		defer file.Close()

		// Parse repodata.json
		var repodata struct {
			Packages map[string]struct {
				Subdir  string `json:"subdir"`
				Version string `json:"version"`
				Name    string `json:"name"`
			} `json:"packages"`
			PackagesConda map[string]struct {
				Subdir  string `json:"subdir"`
				Version string `json:"version"`
				Name    string `json:"name"`
			} `json:"packages.conda"`
		}

		if err := json.NewDecoder(file).Decode(&repodata); err != nil {
			log.Error().Err(err).Msgf("Failed to parse repodata.json: %s", repodataPath)
			continue
		}

		// Create Package entries for each conda package
		for _, pkg := range pkgs {
			// Look up version and subdir from repodata
			packageName := ""
			version := ""
			subdir := ""
			size := pkg.Size

			// Check in packages.conda first (for .conda files), then packages (for .tar.bz2 files)
			if pkgInfo, exists := repodata.PackagesConda[pkg.Name]; exists {
				version = pkgInfo.Version
				subdir = pkgInfo.Subdir
				packageName = pkgInfo.Name
			} else if pkgInfo, exists := repodata.Packages[pkg.Name]; exists {
				version = pkgInfo.Version
				subdir = pkgInfo.Subdir
				packageName = pkgInfo.Name
			}

			// Format version as <subdir>/<version>
			versionStr := fmt.Sprintf("%s/%s", subdir, version)

			packages = append(packages, types.Package{
				Registry: registry,
				Path:     pkg.Path,
				Name:     packageName,
				Version:  versionStr,
				Size:     size,
			})
		}
	}
	return packages, nil
}
