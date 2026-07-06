package util

import (
	"path"
	"sort"
	"strings"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// Conan v2 JFrog storage layout (repo-relative Uri):
//
//	.../{name}/{version}/{user}/{channel}/{rrev}/export/{file}                 -> recipe layer
//	.../{name}/{version}/{user}/{channel}/{rrev}/package/{pkgid}/{prev}/{file} -> package layer
//
// The reference coordinates are located relative to the "export"/"package"
// layer marker rather than a fixed offset, so any leading prefix some layouts
// use (e.g. a "_" root segment) is ignored. "_" is the placeholder JFrog uses
// for an absent user/channel.
const (
	ConanPlaceholder  = "_"
	conanManifestFile = "conanmanifest.txt"
	conanExportMarker = "export"
	conanPkgMarker    = "package"
	conanFilePy       = "conanfile.py"
	conanInfoTxt      = "conaninfo.txt"

	// segments preceding the layer marker: {name}/{version}/{user}/{channel}/{rrev}
	conanRefSegmentsBeforeMarker = 5
)

// conanTarballExtensions are the compression variants Conan uses for its tarballs.
var conanTarballExtensions = map[string]bool{".tgz": true, ".txz": true, ".tzst": true}

// ConanLayer identifies whether a file belongs to the recipe (RREV) or package
// (PKGID/PREV) layer of a Conan reference.
type ConanLayer string

const (
	ConanLayerRecipe  ConanLayer = "recipe"
	ConanLayerPackage ConanLayer = "package"
)

// ConanRef holds the coordinates of a Conan reference (name/version[@user/channel]).
// User/Channel are "_" when absent, matching the on-disk placeholder.
type ConanRef struct {
	Name    string
	Version string
	User    string
	Channel string
}

// BasePath is the repo-relative subtree path of the reference: /name/version/user/channel.
func (r ConanRef) BasePath() string {
	return "/" + r.Name + "/" + r.Version + "/" + r.User + "/" + r.Channel
}

// Display renders the reference as name/version[@user/channel], omitting a
// placeholder user/channel.
func (r ConanRef) Display() string {
	if r.User == ConanPlaceholder && r.Channel == ConanPlaceholder {
		return r.Name + "/" + r.Version
	}
	return r.Name + "/" + r.Version + "@" + r.User + "/" + r.Channel
}

// ConanFileEntry is a single migratable Conan file with the coordinates needed
// to re-upload it to the destination (recipe or package layer).
type ConanFileEntry struct {
	Reference ConanRef
	Layer     ConanLayer
	RRev      string
	PkgID     string
	PRev      string
	FileName  string
	Uri       string
	SHA1      string
	Size      int
}

// ParseConanFileURI parses a repo-relative JFrog Conan v2 Uri into a
// ConanFileEntry. ok is false for paths that are not canonical Conan recipe or
// package files (e.g. JFrog-internal index/metadata files), so callers can skip
// anything the destination would reject.
func ParseConanFileURI(uri string) (ConanFileEntry, bool) {
	trimmed := strings.Trim(uri, "/")
	if trimmed == "" {
		return ConanFileEntry{}, false
	}
	parts := strings.Split(trimmed, "/")

	// Anchor on the layer marker; the five segments before it are the reference
	// (name/version/user/channel/rrev), so any leading prefix is ignored.
	marker := -1
	for i, p := range parts {
		if p == conanExportMarker || p == conanPkgMarker {
			marker = i
			break
		}
	}
	if marker < conanRefSegmentsBeforeMarker {
		return ConanFileEntry{}, false
	}

	ref := ConanRef{
		Name:    parts[marker-5],
		Version: parts[marker-4],
		User:    parts[marker-3],
		Channel: parts[marker-2],
	}
	rrev := parts[marker-1]
	filename := parts[len(parts)-1]

	switch parts[marker] {
	case conanExportMarker:
		// .../{rrev}/export/{file}
		if len(parts) <= marker+1 {
			return ConanFileEntry{}, false
		}
		if !IsConanRecipeFile(filename) {
			return ConanFileEntry{}, false
		}
		return ConanFileEntry{
			Reference: ref,
			Layer:     ConanLayerRecipe,
			RRev:      rrev,
			FileName:  filename,
			Uri:       uri,
		}, true
	case conanPkgMarker:
		// .../{rrev}/package/{pkgid}/{prev}/{file}
		if len(parts) < marker+4 {
			return ConanFileEntry{}, false
		}
		if !IsConanPackageFile(filename) {
			return ConanFileEntry{}, false
		}
		return ConanFileEntry{
			Reference: ref,
			Layer:     ConanLayerPackage,
			RRev:      rrev,
			PkgID:     parts[marker+1],
			PRev:      parts[marker+2],
			FileName:  filename,
			Uri:       uri,
		}, true
	default:
		return ConanFileEntry{}, false
	}
}

// GetConanPackages returns one package per distinct Conan reference found in the
// file list. Each package's Path is the reference subtree so the package job's
// tree node scopes to just that reference's files.
func GetConanPackages(files []*types.File, registry string) []types.Package {
	seen := make(map[string]bool)
	var packages []types.Package
	for _, f := range files {
		if f == nil || f.Folder {
			continue
		}
		entry, ok := ParseConanFileURI(f.Uri)
		if !ok {
			continue
		}
		key := entry.Reference.BasePath()
		if seen[key] {
			continue
		}
		seen[key] = true
		packages = append(packages, types.Package{
			Registry: registry,
			Path:     key,
			Name:     entry.Reference.Display(),
			Size:     -1,
			Metadata: map[string]string{
				"name":    entry.Reference.Name,
				"version": entry.Reference.Version,
				"user":    entry.Reference.User,
				"channel": entry.Reference.Channel,
			},
		})
	}
	return packages
}

// ParseConanEntries converts a reference's files into upload-ready entries,
// ordered so every conanmanifest.txt is uploaded last within its layer/revision
// group (the finalization marker the server expects last).
func ParseConanEntries(files []*types.File) []ConanFileEntry {
	var entries []ConanFileEntry
	for _, f := range files {
		if f == nil || f.Folder {
			continue
		}
		entry, ok := ParseConanFileURI(f.Uri)
		if !ok {
			continue
		}
		entry.SHA1 = f.SHA1
		entry.Size = f.Size
		entries = append(entries, entry)
	}
	sortConanEntries(entries)
	return entries
}

// sortConanEntries orders recipe files before package files, groups by
// revision, and places conanmanifest.txt last within each group.
func sortConanEntries(entries []ConanFileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Layer != b.Layer {
			// recipe layer first
			return a.Layer == ConanLayerRecipe
		}
		aKey := a.RRev + "|" + a.PkgID + "|" + a.PRev
		bKey := b.RRev + "|" + b.PkgID + "|" + b.PRev
		if aKey != bKey {
			return aKey < bKey
		}
		aManifest := a.FileName == conanManifestFile
		bManifest := b.FileName == conanManifestFile
		if aManifest != bManifest {
			// manifest sorts last within the group
			return !aManifest
		}
		return a.FileName < b.FileName
	})
}

// IsConanRecipeFile reports whether name is a canonical recipe-layer file,
// mirroring the destination server's recipe filename rule.
func IsConanRecipeFile(name string) bool {
	name = path.Base(name)
	if name == conanFilePy || name == conanManifestFile {
		return true
	}
	prefix, ok := conanTarballPrefix(name)
	return ok && (prefix == "conan_export" || prefix == "conan_sources")
}

// IsConanPackageFile reports whether name is a canonical package-layer file,
// mirroring the destination server's package filename rule.
func IsConanPackageFile(name string) bool {
	name = path.Base(name)
	if name == conanInfoTxt || name == conanManifestFile {
		return true
	}
	prefix, ok := conanTarballPrefix(name)
	return ok && prefix == "conan_package"
}

// conanTarballPrefix returns the prefix of a "<prefix>.<ext>" tarball when <ext>
// is an allowed Conan compression variant.
func conanTarballPrefix(name string) (string, bool) {
	ext := path.Ext(name)
	if !conanTarballExtensions[ext] {
		return "", false
	}
	return strings.TrimSuffix(name, ext), true
}
