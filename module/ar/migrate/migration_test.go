package migrate

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// setupTempDir changes the working directory to a temporary directory for the
// duration of the test and returns a restore function that must be deferred.
func setupTempDir(t *testing.T) func() {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	return func() { _ = os.Chdir(origDir) }
}

// captureStdout redirects os.Stdout to a buffer, runs fn, and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = origStdout
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stdout pipe: %v", err)
	}
	return string(out)
}

// newTestMigrationService builds a minimal MigrationService suitable for
// testing writeDryRunOutput without real adapters or API clients.
func newTestMigrationService(stats *types.DryRunStats) *MigrationService {
	return &MigrationService{dryRunStats: stats}
}

// findOutputFiles locates the two dry-run JSON files inside outputDir and returns their paths.
func findOutputFiles(t *testing.T, outputDir string) (fileListPath, dirStructPath string) {
	t.Helper()
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("output directory not created: %v", err)
	}
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), "file_list_"):
			fileListPath = filepath.Join(outputDir, e.Name())
		case strings.HasPrefix(e.Name(), "directory_structure_"):
			dirStructPath = filepath.Join(outputDir, e.Name())
		}
	}
	if fileListPath == "" {
		t.Fatal("file_list_*.json not found in output directory")
	}
	if dirStructPath == "" {
		t.Fatal("directory_structure_*.json not found in output directory")
	}
	return
}

// TestWriteDryRunOutput_EmptyStats verifies that writeDryRunOutput creates the
// output directory and both JSON files even when there are no files or directories.
func TestWriteDryRunOutput_EmptyStats(t *testing.T) {
	defer setupTempDir(t)()

	svc := newTestMigrationService(&types.DryRunStats{
		Files:       make([]types.DryRunFileEntry, 0),
		Directories: make(map[string]*types.DryRunDirectoryEntry),
	})

	if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fileListPath, dirStructPath := findOutputFiles(t, "dry-run-output")

	// File list should be an empty JSON array
	data, _ := os.ReadFile(fileListPath)
	var files []types.DryRunFileEntry
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatalf("failed to unmarshal file list: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}

	// Directory structure should be an empty JSON object
	data, _ = os.ReadFile(dirStructPath)
	var dirs map[string]*types.DryRunDirectoryEntry
	if err := json.Unmarshal(data, &dirs); err != nil {
		t.Fatalf("failed to unmarshal directory structure: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 directories, got %d", len(dirs))
	}
}

// TestWriteDryRunOutput_FileListContent verifies that the file_list JSON file
// contains exactly the entries from dryRunStats.Files.
func TestWriteDryRunOutput_FileListContent(t *testing.T) {
	defer setupTempDir(t)()

	inputFiles := []types.DryRunFileEntry{
		{Registry: "reg1", Name: "alpha.jar", Uri: "/reg1/alpha.jar", Size: 1024},
		{Registry: "reg1", Name: "beta.jar", Uri: "/reg1/beta.jar", Size: 2048, LastModified: "2024-01-01"},
	}

	svc := newTestMigrationService(&types.DryRunStats{
		Files:       inputFiles,
		Directories: make(map[string]*types.DryRunDirectoryEntry),
	})

	if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fileListPath, _ := findOutputFiles(t, "dry-run-output")
	data, _ := os.ReadFile(fileListPath)

	var got []types.DryRunFileEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal file list: %v", err)
	}

	if len(got) != len(inputFiles) {
		t.Fatalf("expected %d files, got %d", len(inputFiles), len(got))
	}
	for i, f := range inputFiles {
		if got[i] != f {
			t.Errorf("file[%d] mismatch: want %+v, got %+v", i, f, got[i])
		}
	}
}

// TestWriteDryRunOutput_DirectoryStructureContent verifies that the
// directory_structure JSON file reflects the full registry/package/version
// hierarchy from dryRunStats.Directories.
func TestWriteDryRunOutput_DirectoryStructureContent(t *testing.T) {
	defer setupTempDir(t)()

	inputDirs := map[string]*types.DryRunDirectoryEntry{
		"reg1": {
			Registry: "reg1",
			Packages: map[string]*types.DryRunPackageEntry{
				"com.example:lib": {
					Name: "com.example:lib",
					Versions: map[string]*types.DryRunVersionEntry{
						"1.0.0": {
							Name: "1.0.0",
							Files: []types.DryRunVersionFileEntry{
								{Name: "lib-1.0.0.jar", Registry: "reg1", Uri: "/reg1/lib-1.0.0.jar", Size: 512},
							},
						},
					},
				},
			},
		},
	}

	svc := newTestMigrationService(&types.DryRunStats{
		Files:       []types.DryRunFileEntry{},
		Directories: inputDirs,
	})

	if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, dirStructPath := findOutputFiles(t, "dry-run-output")
	data, _ := os.ReadFile(dirStructPath)

	var got map[string]*types.DryRunDirectoryEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal directory structure: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 registry, got %d", len(got))
	}
	reg, ok := got["reg1"]
	if !ok {
		t.Fatal("registry 'reg1' not found in output")
	}
	if len(reg.Packages) != 1 {
		t.Errorf("expected 1 package, got %d", len(reg.Packages))
	}
	pkg, ok := reg.Packages["com.example:lib"]
	if !ok {
		t.Fatal("package 'com.example:lib' not found in output")
	}
	if len(pkg.Versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(pkg.Versions))
	}
	ver, ok := pkg.Versions["1.0.0"]
	if !ok {
		t.Fatal("version '1.0.0' not found in output")
	}
	if len(ver.Files) != 1 {
		t.Errorf("expected 1 file in version, got %d", len(ver.Files))
	}
}

// TestWriteDryRunOutput_MigratedLabel_UsesFilesWhenFilteredFilesExist verifies
// that the summary line says "Files that passed all filters" when filteredFiles > 0.
func TestWriteDryRunOutput_MigratedLabel_UsesFilesWhenFilteredFilesExist(t *testing.T) {
	defer setupTempDir(t)()

	svc := newTestMigrationService(&types.DryRunStats{
		Files: []types.DryRunFileEntry{
			{Registry: "reg1", Name: "a.txt", Uri: "/a.txt", Size: 10},
		},
		Directories: map[string]*types.DryRunDirectoryEntry{
			"reg1": {
				Registry: "reg1",
				Packages: map[string]*types.DryRunPackageEntry{
					"pkg1": {
						Name: "pkg1",
						Versions: map[string]*types.DryRunVersionEntry{
							"1.0": {
								Name: "1.0",
								Files: []types.DryRunVersionFileEntry{
									{Name: "a.txt", Registry: "reg1", Uri: "/a.txt", Size: 10},
									{Name: "b.txt", Registry: "reg1", Uri: "/b.txt", Size: 20},
								},
							},
						},
					},
				},
			},
		},
	})

	stdout := captureStdout(t, func() {
		if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(stdout, "Files that passed all filters") {
		t.Errorf("expected 'Files that passed all filters' in output, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Packages that passed all filters") {
		t.Errorf("unexpected 'Packages that passed all filters' in output when filteredFiles > 0")
	}
}

// TestWriteDryRunOutput_MigratedLabel_UsesPackagesWhenNoFilteredFiles verifies
// that the summary line says "Packages that passed all filters" when
// filteredFiles == 0 but totalPackages > 0.
func TestWriteDryRunOutput_MigratedLabel_UsesPackagesWhenNoFilteredFiles(t *testing.T) {
	defer setupTempDir(t)()

	svc := newTestMigrationService(&types.DryRunStats{
		Files: []types.DryRunFileEntry{},
		Directories: map[string]*types.DryRunDirectoryEntry{
			"reg1": {
				Registry: "reg1",
				Packages: map[string]*types.DryRunPackageEntry{
					"pkg1": {
						Name:     "pkg1",
						Versions: map[string]*types.DryRunVersionEntry{},
					},
					"pkg2": {
						Name: "pkg2",
						Versions: map[string]*types.DryRunVersionEntry{
							"2.0": {
								Name:  "2.0",
								Files: []types.DryRunVersionFileEntry{},
							},
						},
					},
				},
			},
		},
	})

	stdout := captureStdout(t, func() {
		if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(stdout, "Packages that passed all filters") {
		t.Errorf("expected 'Packages that passed all filters' in output, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Files that passed all filters") {
		t.Errorf("unexpected 'Files that passed all filters' when filteredFiles == 0")
	}
}

// TestWriteDryRunOutput_NilEntriesHandled verifies that nil registry, package,
// and version entries are skipped without panicking and the method returns nil.
func TestWriteDryRunOutput_NilEntriesHandled(t *testing.T) {
	defer setupTempDir(t)()

	svc := newTestMigrationService(&types.DryRunStats{
		Files: []types.DryRunFileEntry{},
		Directories: map[string]*types.DryRunDirectoryEntry{
			"nilReg": nil,
			"goodReg": {
				Registry: "goodReg",
				Packages: map[string]*types.DryRunPackageEntry{
					"nilPkg": nil,
					"goodPkg": {
						Name: "goodPkg",
						Versions: map[string]*types.DryRunVersionEntry{
							"nilVer": nil,
							"goodVer": {
								Name:  "goodVer",
								Files: []types.DryRunVersionFileEntry{},
							},
						},
					},
				},
			},
		},
	})

	if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
		t.Fatalf("unexpected error with nil entries: %v", err)
	}
}

// TestWriteDryRunOutput_SummaryCountsAreCorrect verifies the printed summary
// totals (registries, packages, versions, filtered files) match the input data.
func TestWriteDryRunOutput_SummaryCountsAreCorrect(t *testing.T) {
	defer setupTempDir(t)()

	svc := newTestMigrationService(&types.DryRunStats{
		Files: []types.DryRunFileEntry{
			{Registry: "reg1", Name: "f1.jar", Uri: "/f1.jar", Size: 100},
			{Registry: "reg1", Name: "f2.jar", Uri: "/f2.jar", Size: 200},
			{Registry: "reg1", Name: "f3.jar", Uri: "/f3.jar", Size: 300},
		},
		Directories: map[string]*types.DryRunDirectoryEntry{
			"reg1": {
				Registry: "reg1",
				Packages: map[string]*types.DryRunPackageEntry{
					"pkgA": {
						Name: "pkgA",
						Versions: map[string]*types.DryRunVersionEntry{
							"1.0": {
								Name: "1.0",
								Files: []types.DryRunVersionFileEntry{
									{Name: "f1.jar", Registry: "reg1", Uri: "/f1.jar", Size: 100},
								},
							},
							"2.0": {
								Name: "2.0",
								Files: []types.DryRunVersionFileEntry{
									{Name: "f2.jar", Registry: "reg1", Uri: "/f2.jar", Size: 200},
								},
							},
						},
					},
					"pkgB": {
						Name: "pkgB",
						Versions: map[string]*types.DryRunVersionEntry{
							"1.0": {
								Name: "1.0",
								Files: []types.DryRunVersionFileEntry{
									{Name: "f3.jar", Registry: "reg1", Uri: "/f3.jar", Size: 300},
								},
							},
						},
					},
				},
			},
			"reg2": {
				Registry: "reg2",
				Packages: map[string]*types.DryRunPackageEntry{},
			},
		},
	})

	stdout := captureStdout(t, func() {
		if err := svc.writeDryRunOutput(zerolog.Nop()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	checks := []struct {
		label string
		value string
	}{
		{"Files found in source registry", "3"},
		{"Registries", "2"},
		{"Packages", "2"},
		{"Versions", "3"},
	}
	for _, c := range checks {
		if !strings.Contains(stdout, c.value) {
			t.Errorf("expected count %s for %q in output:\n%s", c.value, c.label, stdout)
		}
	}
}
