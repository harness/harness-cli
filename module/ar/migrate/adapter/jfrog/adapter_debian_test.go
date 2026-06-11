package jfrog

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestParseDebianRelease(t *testing.T) {
	tests := []struct {
		name                  string
		releaseContent        string
		expectedComponents    []string
		expectedArchitectures []string
		expectError           bool
	}{
		{
			name: "Standard Release file",
			releaseContent: `Origin: Debian
Label: Debian
Suite: stable
Codename: bookworm
Components: main contrib non-free
Architectures: amd64 arm64 i386
Date: Sat, 10 Jun 2023 12:00:00 UTC`,
			expectedComponents:    []string{"main", "contrib", "non-free"},
			expectedArchitectures: []string{"amd64", "arm64", "i386"},
			expectError:           false,
		},
		{
			name: "Release file with single component",
			releaseContent: `Origin: Ubuntu
Suite: focal
Components: main
Architectures: amd64`,
			expectedComponents:    []string{"main"},
			expectedArchitectures: []string{"amd64"},
			expectError:           false,
		},
		{
			name: "Release file with extra whitespace",
			releaseContent: `Components:  main   contrib
Architectures:   amd64    arm64   `,
			expectedComponents:    []string{"main", "contrib"},
			expectedArchitectures: []string{"amd64", "arm64"},
			expectError:           false,
		},
		{
			name: "Empty Release file - should error",
			releaseContent: `Origin: Test
Suite: test`,
			expectedComponents:    []string{},
			expectedArchitectures: []string{},
			expectError:           true,
		},
		{
			name: "Release file with only components - should error",
			releaseContent: `Components: main contrib
Date: Sat, 10 Jun 2023 12:00:00 UTC`,
			expectedComponents:    []string{},
			expectedArchitectures: []string{},
			expectError:           true,
		},
		{
			name: "Release file with only architectures - should error",
			releaseContent: `Architectures: amd64 arm64
Date: Sat, 10 Jun 2023 12:00:00 UTC`,
			expectedComponents:    []string{},
			expectedArchitectures: []string{},
			expectError:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.releaseContent)
			components, architectures, err := parseDebianRelease(reader)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !stringSliceEqual(components, tt.expectedComponents) {
				t.Errorf("components: expected %v, got %v", tt.expectedComponents, components)
			}

			if !stringSliceEqual(architectures, tt.expectedArchitectures) {
				t.Errorf("architectures: expected %v, got %v", tt.expectedArchitectures, architectures)
			}
		})
	}
}

func TestExtractDebianPackages(t *testing.T) {
	tests := []struct {
		name             string
		packagesContent  string
		isGzipped        bool
		registry         string
		distribution     string
		component        string
		expectedPackages int
		expectError      bool
	}{
		{
			name: "Single package",
			packagesContent: `Package: nginx
Version: 1.18.0-1
Architecture: amd64
Filename: pool/main/n/nginx/nginx_1.18.0-1_amd64.deb
Size: 1048576
SHA256: abc123

`,
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "bookworm",
			component:        "main",
			expectedPackages: 1,
			expectError:      false,
		},
		{
			name: "Multiple packages",
			packagesContent: `Package: nginx
Version: 1.18.0-1
Architecture: amd64
Filename: pool/main/n/nginx/nginx_1.18.0-1_amd64.deb
Size: 1048576

Package: apache2
Version: 2.4.52-1
Architecture: amd64
Filename: pool/main/a/apache2/apache2_2.4.52-1_amd64.deb
Size: 2097152

`,
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "bookworm",
			component:        "main",
			expectedPackages: 2,
			expectError:      false,
		},
		{
			name:             "Empty packages file",
			packagesContent:  "",
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "bookworm",
			component:        "main",
			expectedPackages: 0,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader *bytes.Reader

			if tt.isGzipped {
				var buf bytes.Buffer
				gw := gzip.NewWriter(&buf)
				gw.Write([]byte(tt.packagesContent))
				gw.Close()
				reader = bytes.NewReader(buf.Bytes())
			} else {
				reader = bytes.NewReader([]byte(tt.packagesContent))
			}

			packages, err := extractDebianPackages(reader, tt.registry, tt.distribution, tt.component, tt.isGzipped)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(packages) != tt.expectedPackages {
				t.Errorf("expected %d packages, got %d", tt.expectedPackages, len(packages))
			}

			// Verify metadata for each package
			for _, pkg := range packages {
				if pkg.Registry != tt.registry {
					t.Errorf("package registry: expected %q, got %q", tt.registry, pkg.Registry)
				}
				if pkg.Metadata["distribution"] != tt.distribution {
					t.Errorf("package distribution: expected %q, got %q", tt.distribution, pkg.Metadata["distribution"])
				}
				if pkg.Metadata["component"] != tt.component {
					t.Errorf("package component: expected %q, got %q", tt.component, pkg.Metadata["component"])
				}
			}
		})
	}
}

func TestExtractDebianSourcePackages(t *testing.T) {
	tests := []struct {
		name             string
		sourcesContent   string
		isGzipped        bool
		registry         string
		distribution     string
		component        string
		expectedPackages int
		expectError      bool
		checkSourceFiles bool
		expectedSources  []string
	}{
		{
			name: "Single source package with multiple files",
			sourcesContent: `Package: apache2
Version: 2.4.52-1
Directory: pool/main/a/apache2
Files:
 5d41402abc4b2a76b9719d911017c592 8388608 apache2_2.4.52.orig.tar.gz
 aaf4c61ddcc5e8a2dabede0f3b482cd9 524288 apache2_2.4.52-1.debian.tar.gz
 e99a18c428cb38d5f260853678922e03 2048 apache2_2.4.52-1.dsc

`,
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "bookworm",
			component:        "main",
			expectedPackages: 1,
			expectError:      false,
			checkSourceFiles: true,
			expectedSources:  []string{"apache2_2.4.52.orig.tar.gz", "apache2_2.4.52-1.debian.tar.gz"},
		},
		{
			name: "Source package with orig.tar only",
			sourcesContent: `Package: nginx
Version: 1.18.0-1
Directory: pool/main/n/nginx
Files:
 abc123 1048576 nginx_1.18.0.orig.tar.xz
 def456 2048 nginx_1.18.0-1.dsc

`,
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "focal",
			component:        "main",
			expectedPackages: 1,
			expectError:      false,
			checkSourceFiles: true,
			expectedSources:  []string{"nginx_1.18.0.orig.tar.xz"},
		},
		{
			name: "Multiple source packages",
			sourcesContent: `Package: apache2
Version: 2.4.52-1
Directory: pool/main/a/apache2
Files:
 abc123 8388608 apache2_2.4.52.orig.tar.gz
 def456 2048 apache2_2.4.52-1.dsc

Package: nginx
Version: 1.18.0-1
Directory: pool/main/n/nginx
Files:
 ghi789 1048576 nginx_1.18.0.orig.tar.xz
 jkl012 2048 nginx_1.18.0-1.dsc

`,
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "bookworm",
			component:        "main",
			expectedPackages: 2,
			expectError:      false,
			checkSourceFiles: false,
		},
		{
			name:             "Empty sources file",
			sourcesContent:   "",
			isGzipped:        false,
			registry:         "debian-local",
			distribution:     "bookworm",
			component:        "main",
			expectedPackages: 0,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader *bytes.Reader

			if tt.isGzipped {
				var buf bytes.Buffer
				gw := gzip.NewWriter(&buf)
				gw.Write([]byte(tt.sourcesContent))
				gw.Close()
				reader = bytes.NewReader(buf.Bytes())
			} else {
				reader = bytes.NewReader([]byte(tt.sourcesContent))
			}

			packages, err := extractDebianSourcePackages(reader, tt.registry, tt.distribution, tt.component, tt.isGzipped)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(packages) != tt.expectedPackages {
				t.Errorf("expected %d packages, got %d", tt.expectedPackages, len(packages))
			}

			// Verify metadata and source files for each package
			for _, pkg := range packages {
				if pkg.Registry != tt.registry {
					t.Errorf("package registry: expected %q, got %q", tt.registry, pkg.Registry)
				}
				if pkg.Metadata["distribution"] != tt.distribution {
					t.Errorf("package distribution: expected %q, got %q", tt.distribution, pkg.Metadata["distribution"])
				}
				if pkg.Metadata["component"] != tt.component {
					t.Errorf("package component: expected %q, got %q", tt.component, pkg.Metadata["component"])
				}

				// Check source files if requested
				if tt.checkSourceFiles {
					sourceFiles := pkg.Metadata["sourceFiles"]
					if sourceFiles == "" {
						t.Errorf("expected sourceFiles metadata but got none")
						continue
					}

					files := strings.Split(sourceFiles, ",")
					if len(files) != len(tt.expectedSources) {
						t.Errorf("expected %d source files, got %d", len(tt.expectedSources), len(files))
					}

					// Check all expected sources are present
					for _, expectedFile := range tt.expectedSources {
						found := false
						for _, actualFile := range files {
							if strings.TrimSpace(actualFile) == expectedFile {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("expected source file %q not found in sourceFiles metadata", expectedFile)
						}
					}
				}
			}
		})
	}
}

// Helper function to compare string slices
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
