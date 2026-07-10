package util

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/stretchr/testify/assert"
)

func TestParseConanFileURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantOK   bool
		layer    ConanLayer
		ref      ConanRef
		rrev     string
		pkgID    string
		prev     string
		fileName string
	}{
		{
			name:     "recipe file without user/channel",
			uri:      "/zlib/1.2.13/_/_/rrev1/export/conanfile.py",
			wantOK:   true,
			layer:    ConanLayerRecipe,
			ref:      ConanRef{Name: "zlib", Version: "1.2.13", User: "_", Channel: "_"},
			rrev:     "rrev1",
			fileName: "conanfile.py",
		},
		{
			name:     "recipe manifest with user/channel",
			uri:      "/mylib/2.0/acme/stable/rrevX/export/conanmanifest.txt",
			wantOK:   true,
			layer:    ConanLayerRecipe,
			ref:      ConanRef{Name: "mylib", Version: "2.0", User: "acme", Channel: "stable"},
			rrev:     "rrevX",
			fileName: "conanmanifest.txt",
		},
		{
			name:     "package file",
			uri:      "/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/conaninfo.txt",
			wantOK:   true,
			layer:    ConanLayerPackage,
			ref:      ConanRef{Name: "zlib", Version: "1.2.13", User: "_", Channel: "_"},
			rrev:     "rrev1",
			pkgID:    "pkgid1",
			prev:     "prev1",
			fileName: "conaninfo.txt",
		},
		{
			name:     "recipe file with leading _ root prefix",
			uri:      "/_/zlib/1.2.13/_/_/rrev1/export/conanfile.py",
			wantOK:   true,
			layer:    ConanLayerRecipe,
			ref:      ConanRef{Name: "zlib", Version: "1.2.13", User: "_", Channel: "_"},
			rrev:     "rrev1",
			fileName: "conanfile.py",
		},
		{
			name:     "package file with leading _ root prefix",
			uri:      "/_/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/conan_package.tgz",
			wantOK:   true,
			layer:    ConanLayerPackage,
			ref:      ConanRef{Name: "zlib", Version: "1.2.13", User: "_", Channel: "_"},
			rrev:     "rrev1",
			pkgID:    "pkgid1",
			prev:     "prev1",
			fileName: "conan_package.tgz",
		},
		{
			name:   "internal index file is skipped",
			uri:    "/zlib/1.2.13/_/_/rrev1/index/.conan_metadata.json",
			wantOK: false,
		},
		{
			name:   "non-canonical recipe filename is skipped",
			uri:    "/zlib/1.2.13/_/_/rrev1/export/random.txt",
			wantOK: false,
		},
		{
			name:   "non-canonical package filename is skipped",
			uri:    "/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/random.bin",
			wantOK: false,
		},
		{
			name:   "too few segments",
			uri:    "/zlib/1.2.13/_/_/rrev1/export",
			wantOK: false,
		},
		{
			name:   "package path missing prev",
			uri:    "/zlib/1.2.13/_/_/rrev1/package/pkgid1/conaninfo.txt",
			wantOK: false,
		},
		{
			name:   "empty uri",
			uri:    "/",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := ParseConanFileURI(tt.uri)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, tt.layer, entry.Layer)
			assert.Equal(t, tt.ref, entry.Reference)
			assert.Equal(t, tt.rrev, entry.RRev)
			assert.Equal(t, tt.pkgID, entry.PkgID)
			assert.Equal(t, tt.prev, entry.PRev)
			assert.Equal(t, tt.fileName, entry.FileName)
			assert.Equal(t, tt.uri, entry.Uri)
		})
	}
}

func TestConanRefDisplayAndBasePath(t *testing.T) {
	noUC := ConanRef{Name: "zlib", Version: "1.2.13", User: "_", Channel: "_"}
	assert.Equal(t, "zlib/1.2.13", noUC.Display())
	assert.Equal(t, "/zlib/1.2.13/_/_", noUC.BasePath())

	withUC := ConanRef{Name: "mylib", Version: "2.0", User: "acme", Channel: "stable"}
	assert.Equal(t, "mylib/2.0@acme/stable", withUC.Display())
	assert.Equal(t, "/mylib/2.0/acme/stable", withUC.BasePath())
}

func TestGetConanPackages(t *testing.T) {
	files := []*types.File{
		{Uri: "/zlib/1.2.13/_/_/rrev1/export/conanfile.py"},
		{Uri: "/zlib/1.2.13/_/_/rrev1/export/conanmanifest.txt"},
		{Uri: "/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/conaninfo.txt"},
		{Uri: "/mylib/2.0/acme/stable/rrevX/export/conanfile.py"},
		{Uri: "/zlib/1.2.13/_/_/rrev1/index/.conan_metadata.json"}, // skipped
		{Uri: "/some/random/file.txt"},                             // skipped
		{Uri: "/dir/", Folder: true},                               // skipped
	}

	packages := GetConanPackages(files, "conan-local")
	assert.Len(t, packages, 2)

	byName := map[string]types.Package{}
	for _, p := range packages {
		byName[p.Name] = p
	}

	zlib, ok := byName["zlib/1.2.13"]
	assert.True(t, ok)
	assert.Equal(t, "/zlib/1.2.13/_/_", zlib.Path)
	assert.Equal(t, "conan-local", zlib.Registry)
	assert.Equal(t, "zlib", zlib.Metadata["name"])
	assert.Equal(t, "1.2.13", zlib.Metadata["version"])
	assert.Equal(t, "_", zlib.Metadata["user"])
	assert.Equal(t, "_", zlib.Metadata["channel"])

	mylib, ok := byName["mylib/2.0@acme/stable"]
	assert.True(t, ok)
	assert.Equal(t, "/mylib/2.0/acme/stable", mylib.Path)
	assert.Equal(t, "acme", mylib.Metadata["user"])
	assert.Equal(t, "stable", mylib.Metadata["channel"])
}

func TestParseConanEntriesOrdering(t *testing.T) {
	// Deliberately unordered: manifests first, package before recipe.
	files := []*types.File{
		{Uri: "/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/conanmanifest.txt", SHA1: "p-man", Size: 1},
		{Uri: "/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/conan_package.tgz", SHA1: "p-tgz", Size: 2},
		{Uri: "/zlib/1.2.13/_/_/rrev1/export/conanmanifest.txt", SHA1: "r-man", Size: 3},
		{Uri: "/zlib/1.2.13/_/_/rrev1/export/conanfile.py", SHA1: "r-py", Size: 4},
		{Uri: "/zlib/1.2.13/_/_/rrev1/package/pkgid1/prev1/conaninfo.txt", SHA1: "p-info", Size: 5},
	}

	entries := ParseConanEntries(files)
	assert.Len(t, entries, 5)

	// Recipe layer comes first, with its manifest last within the group.
	assert.Equal(t, ConanLayerRecipe, entries[0].Layer)
	assert.Equal(t, "conanfile.py", entries[0].FileName)
	assert.Equal(t, "r-py", entries[0].SHA1)
	assert.Equal(t, ConanLayerRecipe, entries[1].Layer)
	assert.Equal(t, "conanmanifest.txt", entries[1].FileName)

	// Then the package layer, again manifest last.
	assert.Equal(t, ConanLayerPackage, entries[2].Layer)
	assert.NotEqual(t, "conanmanifest.txt", entries[2].FileName)
	assert.Equal(t, ConanLayerPackage, entries[3].Layer)
	assert.NotEqual(t, "conanmanifest.txt", entries[3].FileName)
	assert.Equal(t, ConanLayerPackage, entries[4].Layer)
	assert.Equal(t, "conanmanifest.txt", entries[4].FileName)
}

func TestIsConanRecipeFile(t *testing.T) {
	valid := []string{"conanfile.py", "conanmanifest.txt", "conan_export.tgz", "conan_sources.txz", "conan_sources.tzst"}
	for _, n := range valid {
		assert.True(t, IsConanRecipeFile(n), n)
	}
	invalid := []string{"conaninfo.txt", "conan_package.tgz", "random.txt", "conanfile.txt"}
	for _, n := range invalid {
		assert.False(t, IsConanRecipeFile(n), n)
	}
}

func TestIsConanPackageFile(t *testing.T) {
	valid := []string{"conaninfo.txt", "conanmanifest.txt", "conan_package.tgz", "conan_package.txz"}
	for _, n := range valid {
		assert.True(t, IsConanPackageFile(n), n)
	}
	invalid := []string{"conanfile.py", "conan_export.tgz", "conan_sources.tgz", "random.bin"}
	for _, n := range invalid {
		assert.False(t, IsConanPackageFile(n), n)
	}
}
