package types

import (
	"testing"
)

func TestExistingIndex_AddFile(t *testing.T) {
	idx := NewExistingIndex()

	// Add a file with mixed case
	idx.AddFile("mypackage", "1.0.0", "MyFile.TXT")

	// Verify file is found with lowercase query
	if !idx.HasFile("mypackage", "1.0.0", "myfile.txt", "") {
		t.Error("Expected lowercase query to find mixed-case file")
	}

	// Verify file is found with mixed-case query (query is lowercased)
	if !idx.HasFile("mypackage", "1.0.0", "MyFile.TXT", "") {
		t.Error("Expected mixed-case query to find file")
	}
}

func TestExistingIndex_MissingEntries(t *testing.T) {
	idx := NewExistingIndex()
	idx.AddFile("pkg1", "1.0", "file.txt")

	// Missing package
	if idx.HasFile("pkg2", "1.0", "file.txt", "") {
		t.Error("Expected false for missing package file")
	}
	// Missing version
	if idx.HasFile("pkg1", "2.0", "file.txt", "") {
		t.Error("Expected false for missing version file")
	}

	// Missing file
	if idx.HasFile("pkg1", "1.0", "other.txt", "") {
		t.Error("Expected false for missing file")
	}
}

func TestExistingIndex_HasFile_NPMSearchesAllPackages(t *testing.T) {
	idx := NewExistingIndex()
	// The index stores the destination (HAR) path verbatim:
	// "/<package>/<version>/<filename>". HasFile converts it back to the source
	// form "/<package>/-/<filename>" (harToSourcePath) before comparing.
	idx.AddFile("@scope/pkg", "1.0.0", "/@scope/pkg/1.0.0/@scope/pkg-1.0.0.tgz")

	const srcPath = "/@scope/pkg/-/@scope/pkg-1.0.0.tgz"

	// NPM queries key on a pseudo-package/pseudo-version ("") since the source
	// tree flattens all packages and versions; the index is keyed by real HAR
	// package/version pairs, so NPM lookups ignore both and scan every bucket,
	// rewriting each stored HAR path with the bucket's real pkg/version.
	if !idx.HasFile("", "", srcPath, NPM) {
		t.Error("Expected NPM lookup to find file regardless of queried pkg/version")
	}
	if !idx.HasFile("some-other-pkg", "some-other-version", srcPath, NPM) {
		t.Error("Expected NPM lookup to ignore the queried pkg/version entirely")
	}
	if idx.HasFile("@scope/pkg", "1.0.0", "/@scope/pkg/-/missing.tgz", NPM) {
		t.Error("Expected false for a file not present under any package/version")
	}

	// Non-NPM types require an exact pkg/version match AND a matching path;
	// GENERIC does no rewrite, so the source-form query cannot match the stored
	// HAR path.
	if idx.HasFile("", "", srcPath, GENERIC) {
		t.Error("Expected non-NPM lookup to require exact pkg/version match")
	}
}

// TestExistingIndex_HasFile_NuGetStripsPrefix verifies that a NuGet lookup keyed
// by the source-relative path matches a stored HAR path carrying the
// "/<packageID>/<versionID>/" prefix, which harToSourcePath strips at lookup.
func TestExistingIndex_HasFile_NuGet(t *testing.T) {
	idx := NewExistingIndex()
	idx.AddFile("company.grpc.pkg", "1.0.0",
		"/packageID/versionID/foo/company.grpc.pkg.1.0.0.nupkg")
	idx.AddFile("hello.foo.bar.xxx.yyy", "3.203.0-pr-280.a52f7f9.1",
		"/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-pr-280.a52f7f9.1.nupkg")

	if !idx.HasFile("company.grpc.pkg", "1.0.0", "/foo/company.grpc.pkg.1.0.0.nupkg", NUGET) {
		t.Error("Expected NuGet lookup to match after stripping the packageID/versionID prefix")
	}
	if idx.HasFile("company.grpc.pkg", "1.0.0", "/foo/other.nupkg", NUGET) {
		t.Error("Expected false for a file absent from the version")
	}
	// A wrong package/version bucket still matches via the NuGet filename
	// fallback after the normal bucket lookup misses.
	if !idx.HasFile("other.pkg", "1.0.0", "/foo/company.grpc.pkg.1.0.0.nupkg", NUGET) {
		t.Error("Expected filename fallback to match across package/version buckets")
	}

	if !idx.HasFile("hello.foo.bar.xxx.yyy", "3.203.0-pr-280.a52f7f9.1", "/hello/hello.foo.bar.xxx.yyy/3.203.0-INTEGRATION/hello.foo.bar.xxx.yyy.3.203.0-pr-280.a52f7f9.1.nupkg", NUGET) {
		t.Error("Expected true for a mismatched package")
	}
}

func TestExistingIndex_HasFile_NuGetFallsBackToPathAcrossIndex(t *testing.T) {
	idx := NewExistingIndex()
	idx.AddFile(
		"har-derived-package",
		"har-derived-version",
		"/packageID/versionID/uwm/Some Path/My.Package.1.2.3-RC.1.nupkg",
	)

	if !idx.HasFile(
		"source-derived-package",
		"source-derived-version",
		"/uwm/some path/my.package.1.2.3-rc.1.nupkg",
		NUGET,
	) {
		t.Error("expected NuGet path fallback to match across package/version buckets")
	}

	if idx.HasFile(
		"source-derived-package",
		"source-derived-version",
		"/different/source/path/my.package.1.2.3-rc.1.nupkg",
		NUGET,
	) {
		t.Error("path fallback must not match the same filename in another folder")
	}

	if idx.HasFile(
		"source-derived-package",
		"source-derived-version",
		"/uwm/some path/my.package.1.2.3-rc.1.nupkg",
		GENERIC,
	) {
		t.Error("global path fallback must only apply to NuGet")
	}
}

// TestHarToSourcePath verifies that destination (HAR) file paths are converted
// back to the source-relative form used by the migration tree, so the
// existingIndex built from ListFilesV3 matches lookups keyed by file.Uri.
func TestHarToSourcePath(t *testing.T) {
	tests := []struct {
		name         string
		artifactType ArtifactType
		harPath      string
		pkg          string
		version      string
		want         string
	}{
		{
			name:         "nuget strips packageID/versionID prefix",
			artifactType: NUGET,
			harPath:      "/packageID/versionID/foo/company.grpc.pkg/1.0.0/company.grpc.pkg.1.0.0.nupkg",
			want:         "/foo/company.grpc.pkg/1.0.0/company.grpc.pkg.1.0.0.nupkg",
		},
		{
			name:         "nuget flat file",
			artifactType: NUGET,
			harPath:      "/packageID/versionID/company.grpc.pkg.1.0.0.nupkg",
			want:         "/company.grpc.pkg.1.0.0.nupkg",
		},
		{
			name:         "nuget too few segments returned unchanged",
			artifactType: NUGET,
			harPath:      "/packageID/versionID",
			want:         "/packageID/versionID",
		},
		{
			name:         "generic direct match (no rewrite)",
			artifactType: GENERIC,
			harPath:      "/some/file.txt",
			want:         "/some/file.txt",
		},
		{
			name:         "npm scoped package replaces version segment with dash",
			artifactType: NPM,
			harPath:      "/@shiftleft/agent/0.0.1/@shiftleft/agent-0.0.1.tgz",
			pkg:          "@shiftleft/agent",
			version:      "0.0.1",
			want:         "/@shiftleft/agent/-/@shiftleft/agent-0.0.1.tgz",
		},
		{
			name:         "npm unscoped package replaces version segment with dash",
			artifactType: NPM,
			harPath:      "/lodash/4.17.21/lodash-4.17.21.tgz",
			pkg:          "lodash",
			version:      "4.17.21",
			want:         "/lodash/-/lodash-4.17.21.tgz",
		},
		{
			name:         "npm path not matching pkg/version prefix returned unchanged",
			artifactType: NPM,
			harPath:      "/lodash/4.17.21/lodash-4.17.21.tgz",
			pkg:          "lodash",
			version:      "9.9.9",
			want:         "/lodash/4.17.21/lodash-4.17.21.tgz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := harToSourcePath(tt.artifactType, tt.harPath, tt.pkg, tt.version); got != tt.want {
				t.Errorf("harToSourcePath(%q, %q, %q, %q) = %q, want %q",
					tt.artifactType, tt.harPath, tt.pkg, tt.version, got, tt.want)
			}
		})
	}
}

// TestExistingIndex_HasFile_CRANLeadingSlash documents that ListFilesV3 stores
// CRAN/RAW paths with a leading slash, so lookups remapped via CranHarUploadPath
// (no leading slash) must be prefixed before HasFile or the O(1) hit misses.
func TestExistingIndex_HasFile_CRANLeadingSlash(t *testing.T) {
	idx := NewExistingIndex()
	idx.AddFile("jsonlite", "1.7.0", "/src/contrib/jsonlite_1.7.0.tar.gz")

	if !idx.HasFile("jsonlite", "1.7.0", "/src/contrib/jsonlite_1.7.0.tar.gz", CRAN) {
		t.Error("expected leading-slash query to hit ListFilesV3-shaped index key")
	}
	if idx.HasFile("jsonlite", "1.7.0", "src/contrib/jsonlite_1.7.0.tar.gz", CRAN) {
		t.Error("query without leading slash must not match a slash-prefixed index key")
	}
}
