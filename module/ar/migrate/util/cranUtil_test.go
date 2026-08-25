package util

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

func TestIsCranIndexFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"packages plain", "src/contrib/PACKAGES", true},
		{"packages gz", "/src/contrib/PACKAGES.gz", true},
		{"packages rds", "bin/windows/contrib/4.4/PACKAGES.rds", true},
		{"source archive", "src/contrib/jsonlite_1.8.0.tar.gz", false},
		{"leaf only packages", "PACKAGES", true},
		{"archived packages index", "src/contrib/Archive/jsonlite/PACKAGES", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCranIndexFile(tt.path); got != tt.want {
				t.Fatalf("IsCranIndexFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseCranUploadPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantType      string
		wantOS        string
		wantArch      string
		wantRVer      string
		wantRepo      string
		wantExt       string
		wantDest      string
		wantErr       bool
	}{
		{
			name:     "source",
			path:     "src/contrib/jsonlite_1.8.0.tar.gz",
			wantType: "source",
			wantRepo: "src/contrib",
			wantExt:  ".tar.gz",
			wantDest: "src/contrib/jsonlite_1.8.0.tar.gz",
		},
		{
			name:     "source with leading slash",
			path:     "/src/contrib/jsonlite_1.8.0.tar.gz",
			wantType: "source",
			wantRepo: "src/contrib",
			wantExt:  ".tar.gz",
			wantDest: "src/contrib/jsonlite_1.8.0.tar.gz",
		},
		{
			name:         "source archived",
			path:         "src/contrib/Archive/jsonlite/jsonlite_1.7.0.tar.gz",
			wantType:     "source",
			wantRepo:     "src/contrib",
			wantExt:      ".tar.gz",
			wantDest:     "src/contrib/jsonlite_1.7.0.tar.gz",
		},
		{
			name:         "jfrog source archived with version dir",
			path:         "src/contrib/Archive/jsonlite/1.8.8/jsonlite_1.8.8.tar.gz",
			wantType:     "source",
			wantRepo:     "src/contrib",
			wantExt:      ".tar.gz",
			wantDest:     "src/contrib/jsonlite_1.8.8.tar.gz",
		},
		{
			name:     "windows binary",
			path:     "bin/windows/contrib/4.4/jsonlite_1.8.0.zip",
			wantType: "binary",
			wantOS:   "windows",
			wantRVer: "4.4",
			wantRepo: "bin/windows/contrib/4.4",
			wantExt:  ".zip",
			wantDest: "bin/windows/contrib/4.4/jsonlite_1.8.0.zip",
		},
		{
			name:         "windows archived",
			path:         "bin/windows/contrib/4.4/Archive/jsonlite/jsonlite_1.7.0.zip",
			wantType:     "binary",
			wantOS:       "windows",
			wantRVer:     "4.4",
			wantRepo:     "bin/windows/contrib/4.4",
			wantExt:      ".zip",
			wantDest:     "bin/windows/contrib/4.4/jsonlite_1.7.0.zip",
		},
		{
			name:         "jfrog windows archived with version dir",
			path:         "bin/windows/contrib/4.4/Archive/jsonlite/1.7.0/jsonlite_1.7.0.zip",
			wantType:     "binary",
			wantOS:       "windows",
			wantRVer:     "4.4",
			wantRepo:     "bin/windows/contrib/4.4",
			wantExt:      ".zip",
			wantDest:     "bin/windows/contrib/4.4/jsonlite_1.7.0.zip",
		},
		{
			name:     "macos legacy",
			path:     "bin/macosx/contrib/4.4/jsonlite_1.8.0.tgz",
			wantType: "binary",
			wantOS:   "macosx",
			wantRVer: "4.4",
			wantRepo: "bin/macosx/contrib/4.4",
			wantExt:  ".tgz",
			wantDest: "bin/macosx/contrib/4.4/jsonlite_1.8.0.tgz",
		},
		{
			name:         "macos legacy archived",
			path:         "bin/macosx/contrib/4.4/Archive/jsonlite/jsonlite_1.7.0.tgz",
			wantType:     "binary",
			wantOS:       "macosx",
			wantRVer:     "4.4",
			wantRepo:     "bin/macosx/contrib/4.4",
			wantExt:      ".tgz",
			wantDest:     "bin/macosx/contrib/4.4/jsonlite_1.7.0.tgz",
		},
		{
			name:     "macos flavored",
			path:     "bin/macosx/big-sur-arm64/contrib/4.4/jsonlite_1.8.0.tgz",
			wantType: "binary",
			wantOS:   "macosx",
			wantArch: "big-sur-arm64",
			wantRVer: "4.4",
			wantRepo: "bin/macosx/big-sur-arm64/contrib/4.4",
			wantExt:  ".tgz",
			wantDest: "bin/macosx/big-sur-arm64/contrib/4.4/jsonlite_1.8.0.tgz",
		},
		{
			name:         "macos flavored archived",
			path:         "bin/macosx/big-sur-arm64/contrib/4.4/Archive/jsonlite/jsonlite_1.7.0.tgz",
			wantType:     "binary",
			wantOS:       "macosx",
			wantArch:     "big-sur-arm64",
			wantRVer:     "4.4",
			wantRepo:     "bin/macosx/big-sur-arm64/contrib/4.4",
			wantExt:      ".tgz",
			wantDest:     "bin/macosx/big-sur-arm64/contrib/4.4/jsonlite_1.7.0.tgz",
		},
		{
			name:     "jfrog macos without macosx segment",
			path:     "bin/big-sur-arm64/contrib/4.4/jsonlite_2.0.0.tgz",
			wantType: "binary",
			wantOS:   "macosx",
			wantArch: "big-sur-arm64",
			wantRVer: "4.4",
			wantRepo: "bin/macosx/big-sur-arm64/contrib/4.4",
			wantExt:  ".tgz",
			wantDest: "bin/macosx/big-sur-arm64/contrib/4.4/jsonlite_2.0.0.tgz",
		},
		{
			name:     "jfrog macos x86_64 without macosx segment",
			path:     "bin/big-sur-x86_64/contrib/4.4/jsonlite_2.0.0.tgz",
			wantType: "binary",
			wantOS:   "macosx",
			wantArch: "big-sur-x86_64",
			wantRVer: "4.4",
			wantRepo: "bin/macosx/big-sur-x86_64/contrib/4.4",
			wantExt:  ".tgz",
			wantDest: "bin/macosx/big-sur-x86_64/contrib/4.4/jsonlite_2.0.0.tgz",
		},
		{
			name:     "jfrog macos unknown codename still remaps",
			path:     "bin/mojave/contrib/4.4/jsonlite_2.0.0.tgz",
			wantType: "binary",
			wantOS:   "macosx",
			wantArch: "mojave",
			wantRVer: "4.4",
			wantRepo: "bin/macosx/mojave/contrib/4.4",
			wantExt:  ".tgz",
			wantDest: "bin/macosx/mojave/contrib/4.4/jsonlite_2.0.0.tgz",
		},
		{
			name:         "jfrog macos archived with version dir",
			path:         "bin/big-sur-arm64/contrib/4.4/Archive/jsonlite/1.7.0/jsonlite_1.7.0.tgz",
			wantType:     "binary",
			wantOS:       "macosx",
			wantArch:     "big-sur-arm64",
			wantRVer:     "4.4",
			wantRepo:     "bin/macosx/big-sur-arm64/contrib/4.4",
			wantExt:      ".tgz",
			wantDest:     "bin/macosx/big-sur-arm64/contrib/4.4/jsonlite_1.7.0.tgz",
		},
		{name: "empty", path: "", wantErr: true},
		{name: "wrong root", path: "lib/contrib/pkg_1.0.tar.gz", wantErr: true},
		{name: "source wrong ext", path: "src/contrib/pkg_1.0.zip", wantErr: true},
		{name: "source wrong depth", path: "src/pkg_1.0.tar.gz", wantErr: true},
		{name: "windows bad r version", path: "bin/windows/contrib/4/pkg_1.0.zip", wantErr: true},
		{name: "archive pkg dir mismatch", path: "src/contrib/Archive/other/jsonlite_1.7.0.tar.gz", wantErr: true},
		{name: "archive invalid pkg dir", path: "src/contrib/Archive/1bad/jsonlite_1.7.0.tar.gz", wantErr: true},
		{name: "jfrog archive version dir mismatch", path: "src/contrib/Archive/jsonlite/9.9.9/jsonlite_1.8.8.tar.gz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCranUploadPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Type != tt.wantType || got.OS != tt.wantOS || got.Arch != tt.wantArch ||
				got.RVersion != tt.wantRVer || got.RepoPath != tt.wantRepo || got.Extension != tt.wantExt {
				t.Fatalf("got %+v, want type=%s os=%s arch=%s r=%s repo=%s ext=%s",
					got, tt.wantType, tt.wantOS, tt.wantArch, tt.wantRVer, tt.wantRepo, tt.wantExt)
			}
			if dest := got.DestUploadPath(); dest != tt.wantDest {
				t.Fatalf("DestUploadPath() = %q, want %q", dest, tt.wantDest)
			}
		})
	}
}

func TestParseCranPackageFileName(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		ext         string
		wantName    string
		wantVersion string
		wantErr     bool
	}{
		{"source", "jsonlite_1.8.0.tar.gz", ".tar.gz", "jsonlite", "1.8.0", false},
		{"dotted name", "data.table_1.14.8.tar.gz", ".tar.gz", "data.table", "1.14.8", false},
		{"windows zip", "jsonlite_1.8.0.zip", ".zip", "jsonlite", "1.8.0", false},
		{"missing underscore", "jsonlite1.8.0.tar.gz", ".tar.gz", "", "", true},
		{"invalid name", "1bad_1.0.0.tar.gz", ".tar.gz", "", "", true},
		{"empty version", "jsonlite_.tar.gz", ".tar.gz", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVer, err := ParseCranPackageFileName(tt.fileName, tt.ext)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q %q", gotName, gotVer)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotName != tt.wantName || gotVer != tt.wantVersion {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotName, gotVer, tt.wantName, tt.wantVersion)
			}
		})
	}
}

func TestParseCranFileNameWithPath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
		wantOK      bool
	}{
		{"source layout", "/src/contrib/jsonlite_1.8.0.tar.gz", "jsonlite", "1.8.0", true},
		{"source archived", "src/contrib/Archive/jsonlite/jsonlite_1.7.0.tar.gz", "jsonlite", "1.7.0", true},
		{"jfrog source archived", "src/contrib/Archive/jsonlite/1.8.8/jsonlite_1.8.8.tar.gz", "jsonlite", "1.8.8", true},
		{"windows layout", "bin/windows/contrib/4.4/jsonlite_1.8.0.zip", "jsonlite", "1.8.0", true},
		{"windows archived", "bin/windows/contrib/4.4/Archive/jsonlite/jsonlite_1.7.0.zip", "jsonlite", "1.7.0", true},
		{"macos flavored", "bin/macosx/big-sur-arm64/contrib/4.3/data.table_1.14.8.tgz", "data.table", "1.14.8", true},
		{"macos flavored archived", "bin/macosx/big-sur-arm64/contrib/4.3/Archive/data.table/data.table_1.14.0.tgz", "data.table", "1.14.0", true},
		{"jfrog macos", "bin/big-sur-arm64/contrib/4.4/jsonlite_2.0.0.tgz", "jsonlite", "2.0.0", true},
		{"index skipped", "src/contrib/PACKAGES.gz", "", "", false},
		{"non cran path", "/some/other/jsonlite_1.8.0.tar.gz", "", "", false},
		{"bad filename", "src/contrib/not-a-package.tar.gz", "", "", false},
		{"archive dir mismatch", "src/contrib/Archive/other/jsonlite_1.7.0.tar.gz", "", "", false},
		{"jfrog archive version mismatch", "src/contrib/Archive/jsonlite/9.9.9/jsonlite_1.8.8.tar.gz", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVer, ok := ParseCranFileNameWithPath(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %q %q)", ok, tt.wantOK, gotName, gotVer)
			}
			if gotName != tt.wantName || gotVer != tt.wantVersion {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotName, gotVer, tt.wantName, tt.wantVersion)
			}
		})
	}
}

func TestCranHarUploadPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
		ok   bool
	}{
		{"live source", "/src/contrib/jsonlite_1.8.0.tar.gz", "src/contrib/jsonlite_1.8.0.tar.gz", true},
		{"archived source", "src/contrib/Archive/jsonlite/jsonlite_1.7.0.tar.gz", "src/contrib/jsonlite_1.7.0.tar.gz", true},
		{"jfrog archived source", "src/contrib/Archive/jsonlite/1.8.8/jsonlite_1.8.8.tar.gz",
			"src/contrib/jsonlite_1.8.8.tar.gz", true},
		{"archived windows", "bin/windows/contrib/4.4/Archive/jsonlite/jsonlite_1.7.0.zip",
			"bin/windows/contrib/4.4/jsonlite_1.7.0.zip", true},
		{"jfrog macos remap", "bin/big-sur-arm64/contrib/4.4/jsonlite_2.0.0.tgz",
			"bin/macosx/big-sur-arm64/contrib/4.4/jsonlite_2.0.0.tgz", true},
		{"index", "src/contrib/PACKAGES", "", false},
		{"garbage", "foo/bar", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CranHarUploadPath(tt.path)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("CranHarUploadPath(%q) = (%q, %v), want (%q, %v)", tt.path, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseCranUploadPathErrorCases(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"windows wrong extension", "bin/windows/contrib/4.4/jsonlite_1.8.0.tar.gz"},
		{"windows bad layout", "bin/windows/jsonlite_1.8.0.zip"},
		{"windows bad r version", "bin/windows/contrib/notaver/jsonlite_1.8.0.zip"},
		{"macos wrong extension", "bin/macosx/contrib/4.4/jsonlite_1.8.0.zip"},
		{"macos bad flavor", "bin/macosx/BAD_FLAVOR/contrib/4.4/jsonlite_1.8.0.tgz"},
		{"jfrog macos wrong extension", "bin/big-sur-arm64/contrib/4.4/jsonlite_1.8.0.zip"},
		{"jfrog macos bad r version", "bin/big-sur-arm64/contrib/x/jsonlite_1.8.0.tgz"},
		{"bin linux not macos", "bin/linux/contrib/4.4/jsonlite_1.8.0.tgz"},
		{"bin without contrib shape", "bin/mojave/not-contrib/4.4/jsonlite_1.8.0.tgz"},
		{"source wrong extension", "src/contrib/jsonlite_1.8.0.zip"},
		{"source archive pkg mismatch", "src/contrib/Archive/jsonlite/data.table_1.0.0.tar.gz"},
		{"source archive version mismatch", "src/contrib/Archive/jsonlite/9.9.9/jsonlite_1.0.0.tar.gz"},
		{"empty", ""},
		{"unknown root", "lib/contrib/jsonlite_1.8.0.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCranUploadPath(tt.path); err == nil {
				t.Fatalf("ParseCranUploadPath(%q) expected error", tt.path)
			}
		})
	}
}

func TestParseCranUploadPathMacArchiveLayouts(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantRepo string
		wantArch string
	}{
		{
			name:     "classic macosx with flavor archived",
			path:     "bin/macosx/big-sur-arm64/contrib/4.4/Archive/jsonlite/jsonlite_1.0.0.tgz",
			wantRepo: "bin/macosx/big-sur-arm64/contrib/4.4",
			wantArch: "big-sur-arm64",
		},
		{
			name:     "classic macosx with flavor archived versioned",
			path:     "bin/macosx/big-sur-arm64/contrib/4.4/Archive/jsonlite/1.0.0/jsonlite_1.0.0.tgz",
			wantRepo: "bin/macosx/big-sur-arm64/contrib/4.4",
			wantArch: "big-sur-arm64",
		},
		{
			name:     "classic macosx no flavor archived",
			path:     "bin/macosx/contrib/4.4/Archive/jsonlite/jsonlite_1.0.0.tgz",
			wantRepo: "bin/macosx/contrib/4.4",
		},
		{
			name:     "classic macosx no flavor archived versioned",
			path:     "bin/macosx/contrib/4.4/Archive/jsonlite/1.0.0/jsonlite_1.0.0.tgz",
			wantRepo: "bin/macosx/contrib/4.4",
		},
		{
			name:     "jfrog macos archived",
			path:     "bin/big-sur-arm64/contrib/4.4/Archive/jsonlite/jsonlite_1.0.0.tgz",
			wantRepo: "bin/macosx/big-sur-arm64/contrib/4.4",
			wantArch: "big-sur-arm64",
		},
		{
			name:     "jfrog macos archived versioned",
			path:     "bin/big-sur-arm64/contrib/4.4/Archive/jsonlite/1.0.0/jsonlite_1.0.0.tgz",
			wantRepo: "bin/macosx/big-sur-arm64/contrib/4.4",
			wantArch: "big-sur-arm64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseCranUploadPath(tt.path)
			if err != nil {
				t.Fatalf("ParseCranUploadPath: %v", err)
			}
			if info.RepoPath != tt.wantRepo {
				t.Errorf("RepoPath = %q, want %q", info.RepoPath, tt.wantRepo)
			}
			if info.Arch != tt.wantArch {
				t.Errorf("Arch = %q, want %q", info.Arch, tt.wantArch)
			}
			wantDest := tt.wantRepo + "/jsonlite_1.0.0.tgz"
			if info.DestUploadPath() != wantDest {
				t.Errorf("DestUploadPath = %q, want %q", info.DestUploadPath(), wantDest)
			}
		})
	}
}

func TestParseCranUploadPathMacArchiveErrors(t *testing.T) {
	tests := []string{
		"bin/macosx/BAD/contrib/4.4/Archive/jsonlite/jsonlite_1.0.0.tgz",
		"bin/macosx/big-sur-arm64/contrib/4.4/Archive/jsonlite/data.table_1.0.0.tgz",
		"bin/macosx/contrib/4.4/Archive/jsonlite/1.0.0/jsonlite_9.9.9.tgz",
		"bin/big-sur-arm64/contrib/4.4/Archive/jsonlite/data.table_1.0.0.tgz",
		"bin/macosx/unsupported/path/structure.tgz",
		"bin/big-sur-arm64/not-contrib/4.4/jsonlite_1.0.0.tgz",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			if _, err := ParseCranUploadPath(path); err == nil {
				t.Fatalf("expected error for %q", path)
			}
		})
	}
}

func TestParseCranPackageFileNameErrors(t *testing.T) {
	tests := []struct {
		name string
		file string
		ext  string
	}{
		{"missing underscore", "jsonlite.tar.gz", ".tar.gz"},
		{"empty version", "jsonlite_.tar.gz", ".tar.gz"},
		{"bad package name", "1bad_1.0.0.tar.gz", ".tar.gz"},
		{"leading underscore empty name", "_1.0.0.tar.gz", ".tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseCranPackageFileName(tt.file, tt.ext); err == nil {
				t.Fatalf("ParseCranPackageFileName(%q, %q) expected error", tt.file, tt.ext)
			}
		})
	}
}

func TestDestUploadPathNil(t *testing.T) {
	var p *CranPathInfo
	if got := p.DestUploadPath(); got != "" {
		t.Fatalf("DestUploadPath() = %q, want empty", got)
	}
}

func TestBuildCranPackageFilesMap(t *testing.T) {
	files := []types.File{
		{Uri: "/src/contrib/jsonlite_1.8.0.tar.gz"},
		{Uri: "/bin/windows/contrib/4.4/jsonlite_1.8.0.zip"},
		{Uri: "/src/contrib/data.table_1.14.0.tar.gz"},
		{Uri: "/src/contrib/PACKAGES"},
		{Uri: "/not/a/cran/path.txt"},
	}
	got := BuildCranPackageFilesMap(files)
	if len(got["jsonlite"]) != 2 {
		t.Fatalf("jsonlite files = %d, want 2", len(got["jsonlite"]))
	}
	if len(got["data.table"]) != 1 {
		t.Fatalf("data.table files = %d, want 1", len(got["data.table"]))
	}
	if _, ok := got[""]; ok {
		t.Fatal("unexpected empty package key")
	}
}
