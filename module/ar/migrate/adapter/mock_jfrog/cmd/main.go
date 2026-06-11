// Command generate-mock-data creates all fixture files (text content and
// binary packages) in testdata/binary/ so they can be embedded by the mock
// client at compile time.
//
// Usage:
//
//	go run ./module/ar/migrate/adapter/mock_jfrog/cmd
//
// Or via Makefile:
//
//	make mock-init
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
)

var baseDir = filepath.Join("module", "ar", "migrate", "adapter", "mock_jfrog", "testdata", "binary")

func main() {
	entries := []struct {
		path    string
		content []byte
	}{
		// ── Text content fixtures (served by GetFile) ──

		// Maven / multi-purpose
		{"content/maven-local/.pypi/simple.html",
			[]byte(`<html><body><a href="requests/">requests</a><br/><a href="flask/">flask</a><br/></body></html>`)},
		{"content/maven-local/repodata/repomd.xml",
			[]byte(`<?xml version="1.0" encoding="UTF-8"?><repomd><data type="primary"><location href="repodata/primary.xml.gz"/></data></repomd>`)},
		{"content/maven-local/index.yaml",
			[]byte("apiVersion: v1\nentries:\n  nginx:\n    - name: nginx\n      version: 1.0.0\n      urls:\n        - charts/nginx-1.0.0.tgz\n")},

		// Helm legacy
		{"content/helm-legacy-local/index.yaml",
			[]byte("apiVersion: v1\nentries:\n  nginx:\n    - name: nginx\n      version: 8.2.0\n      urls:\n        - nginx-8.2.0.tgz\n")},

		// Dart
		{"content/dart-local/sample_dart_pkg", []byte(`{
  "name": "sample_dart_pkg",
  "latest": {
    "version": "1.1.0",
    "pubspec": {
      "name": "sample_dart_pkg",
      "version": "1.1.0",
      "description": "A sample Dart package for testing migration",
      "homepage": "https://github.com/example/sample_dart_pkg",
      "environment": { "sdk": ">=2.17.0 <4.0.0" }
    },
    "archive_url": "http://localhost:8081/artifactory/dart-local/packages/sample_dart_pkg/versions/1.1.0.tar.gz"
  },
  "versions": [
    { "version": "1.0.0", "pubspec": { "name": "sample_dart_pkg", "version": "1.0.0", "description": "A sample Dart package for testing migration" }, "archive_url": "http://localhost:8081/artifactory/dart-local/packages/sample_dart_pkg/versions/1.0.0.tar.gz" },
    { "version": "1.1.0", "pubspec": { "name": "sample_dart_pkg", "version": "1.1.0", "description": "A sample Dart package for testing migration" }, "archive_url": "http://localhost:8081/artifactory/dart-local/packages/sample_dart_pkg/versions/1.1.0.tar.gz" }
  ]
}`)},

		// NPM metadata
		{"content/npm-local/@har/sample-package", []byte(`{
  "name": "@har/sample-package",
  "description": "A sample NPM package for testing migration",
  "dist-tags": { "latest": "2.0.0", "beta": "2.0.0" },
  "versions": {
    "1.0.0": { "name": "@har/sample-package", "version": "1.0.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-1.0.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80707" } },
    "1.1.0": { "name": "@har/sample-package", "version": "1.1.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-1.1.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80711" } },
    "2.0.0": { "name": "@har/sample-package", "version": "2.0.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-2.0.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80712" } },
    "2.0.0-beta.1": { "name": "@har/sample-package", "version": "2.0.0-beta.1", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-2.0.0-beta.1.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80713" } },
    "3.0.0-rc.1": { "name": "@har/sample-package", "version": "3.0.0-rc.1", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/sample-package/-/sample-package-3.0.0-rc.1.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80714" } }
  },
  "time": { "created": "2023-01-01T00:00:00.000Z", "modified": "2023-05-01T00:00:00.000Z" }
}`)},

		{"content/npm-local/lodash", []byte(`{
  "name": "lodash",
  "description": "Lodash modular utilities",
  "dist-tags": { "latest": "4.17.21", "alpha": "4.17.21-alpha.0" },
  "versions": {
    "4.17.21-alpha.0": { "name": "lodash", "version": "4.17.21-alpha.0", "dist": { "tarball": "http://localhost:8081/artifactory/npm-local/lodash/-/lodash-4.17.21-alpha.0.tgz", "shasum": "da39a3ee5e6b4b0d3255bfef95601890afd80715" } }
  },
  "time": { "created": "2023-06-01T00:00:00.000Z", "modified": "2023-06-01T00:00:00.000Z" }
}`)},

		// Python
		{"content/python-local/.pypi/simple.html",
			[]byte(`<html><body><a href="requests/">requests</a><br/></body></html>`)},
		{"content/python-local/.pypi/requests/requests.html",
			[]byte(`<html><body><a href="../requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a><br/><a href="../requests-2.29.0.tar.gz#sha256=def456">requests-2.29.0.tar.gz</a><br/></body></html>`)},

		// Raw registry — plain files, no package/version structure
		{"content/raw-local/docs/readme.txt",
			[]byte("# Raw Registry\nSample readme for migration testing.\n")},
		{"content/raw-local/configs/v1/config.yaml",
			[]byte("server:\n  host: localhost\n  port: 8080\n")},
		{"content/raw-local/releases/app-1.0.tar.gz",
			[]byte("fake-tarball-content-for-testing")},
		{"content/raw-local/assets/images/logo.png",
			[]byte("fake-png-content-for-testing")},

		// Debian repository metadata
		{"content/debian-local/dists/bookworm/InRelease", []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA512

Suite: bookworm
Codename: bookworm
Components: main
Architectures: amd64 arm64
Date: Sat, 01 Jan 2023 00:00:00 UTC
-----BEGIN PGP SIGNATURE-----

iQIzBAEBCgAdFiEEtest...
-----END PGP SIGNATURE-----`)},

		{"content/debian-local/dists/bookworm/Release", []byte(`Suite: bookworm
Codename: bookworm
Components: main
Architectures: amd64 arm64
Date: Sat, 01 Jan 2023 00:00:00 UTC`)},

		{"content/debian-local/dists/bookworm/main/binary-amd64/Packages.gz", gzipContent([]byte(`Package: nginx
Version: 1.18.0-1
Architecture: amd64
Maintainer: Test Maintainer <test@example.com>
Installed-Size: 1024
Depends: libc6 (>= 2.14)
Section: httpd
Priority: optional
Homepage: https://nginx.org
Description: high performance web server
Filename: pool/main/n/nginx/nginx_1.18.0-1_amd64.deb
Size: 1048576
MD5sum: 5d41402abc4b2a76b9719d911017c592
SHA1: da39a3ee5e6b4b0d3255bfef95601890afd80743
SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855

`))},

		{"content/debian-local/dists/bookworm/main/source/Sources.gz", gzipContent([]byte(`Package: apache2
Binary: apache2, apache2-bin, apache2-data
Version: 2.4.52-1
Maintainer: Debian Apache Maintainers <debian-apache@lists.debian.org>
Architecture: any all
Standards-Version: 4.6.0
Build-Depends: debhelper (>= 13)
Homepage: https://httpd.apache.org/
Directory: pool/main/a/apache2
Files:
 5d41402abc4b2a76b9719d911017c592 8388608 apache2_2.4.52.orig.tar.gz
 aaf4c61ddcc5e8a2dabede0f3b482cd9 524288 apache2_2.4.52-1.debian.tar.gz
 e99a18c428cb38d5f260853678922e03 2048 apache2_2.4.52-1.dsc

`))},

		// ── Binary packages ──

		// NPM tarballs
		{"npm-local/@har/sample-package/-/@har/sample-package-1.0.0.tgz", npmTgz("@har/sample-package", "1.0.0")},
		{"npm-local/@har/sample-package/-/@har/sample-package-1.1.0.tgz", npmTgz("@har/sample-package", "1.1.0")},
		{"npm-local/@har/sample-package/-/@har/sample-package-2.0.0.tgz", npmTgz("@har/sample-package", "2.0.0")},
		{"npm-local/@har/sample-package/-/@har/sample-package-2.0.0-beta.1.tgz", npmTgz("@har/sample-package", "2.0.0-beta.1")},
		{"npm-local/@har/sample-package/-/@har/sample-package-3.0.0-rc.1.tgz", npmTgz("@har/sample-package", "3.0.0-rc.1")},
		{"npm-local/lodash/-/lodash-4.17.21-alpha.0.tgz", npmTgz("lodash", "4.17.21-alpha.0")},

		// Dart tarballs
		{"dart-local/packages/sample_dart_pkg/versions/1.0.0.tar.gz", dartTarGz("sample_dart_pkg", "1.0.0")},
		{"dart-local/packages/sample_dart_pkg/versions/1.1.0.tar.gz", dartTarGz("sample_dart_pkg", "1.1.0")},
		{"dart-local/packages/another_dart_pkg/versions/2.0.0.tar.gz", dartTarGz("another_dart_pkg", "2.0.0")},

		// NuGet packages
		{"nuget-local/foo/company.grpc.pkg/1.0.0/company.grpc.pkg.1.0.0.nupkg", nupkg("company.grpc.pkg", "1.0.0")},
		{"nuget-local/foo/company.grpc.pkg/2.0.0/company.grpc.pkg.2.0.0.nupkg", nupkg("company.grpc.pkg", "2.0.0")},
		{"nuget-local/foo/company.grpc.pkg/2.0.0/company.grpc.pkg.2.0.0.snupkg", nupkg("company.grpc.pkg", "2.0.0")},

		// Debian packages
		{"debian-local/pool/main/n/nginx/nginx_1.18.0-1_amd64.deb", debPackage("nginx", "1.18.0-1", "amd64")},
		{"debian-local/pool/main/a/apache2/apache2_2.4.52-1.dsc", debSourceDsc("apache2", "2.4.52-1")},
		{"debian-local/pool/main/a/apache2/apache2_2.4.52.orig.tar.gz", debSourceTarGz("apache2", "2.4.52")},
		{"debian-local/pool/main/a/apache2/apache2_2.4.52-1.debian.tar.gz", debSourceTarGz("apache2", "2.4.52-1")},
	}

	for _, e := range entries {
		dst := filepath.Join(baseDir, e.path)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(dst), err)
			os.Exit(1)
		}
		if err := os.WriteFile(dst, e.content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", dst, err)
			os.Exit(1)
		}
		fmt.Printf("  wrote %s (%d bytes)\n", dst, len(e.content))
	}
	fmt.Println("mock-init: done")
}

func npmTgz(name, version string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	pkg := fmt.Sprintf(`{"name":%q,"version":%q,"license":"MIT"}`, name, version)
	writetar(tw, "package/package.json", pkg)

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func dartTarGz(name, version string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	pubspec := fmt.Sprintf("name: %s\nversion: %s\n", name, version)
	writetar(tw, "pubspec.yaml", pubspec)

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func nupkg(id, version string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	nuspec := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <id>%s</id>
    <version>%s</version>
    <authors>test</authors>
    <description>Mock NuGet package for migration testing</description>
  </metadata>
</package>`, id, version)

	f, _ := w.Create(id + ".nuspec")
	f.Write([]byte(nuspec))
	w.Close()
	return buf.Bytes()
}

func writetar(tw *tar.Writer, name, content string) {
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))})
	tw.Write([]byte(content))
}

func gzipContent(content []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(content)
	gw.Close()
	return buf.Bytes()
}

func debPackage(name, version, arch string) []byte {
	// Simplified .deb ar archive structure
	var buf bytes.Buffer
	buf.WriteString("!<arch>\n")

	// debian-binary
	debianBinary := "2.0\n"
	writeArFile(&buf, "debian-binary", []byte(debianBinary))

	// control.tar.gz
	controlContent := fmt.Sprintf(`Package: %s
Version: %s
Architecture: %s
Maintainer: Test Maintainer <test@example.com>
Description: Mock Debian package
`, name, version, arch)
	controlTar := createTarGz("control", controlContent)
	writeArFile(&buf, "control.tar.gz", controlTar)

	// data.tar.gz (empty)
	dataTar := createTarGz("", "")
	writeArFile(&buf, "data.tar.gz", dataTar)

	return buf.Bytes()
}

func writeArFile(buf *bytes.Buffer, filename string, data []byte) {
	header := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8s%-10d`\n",
		filename, 1234567890, 0, 0, "100644", len(data))
	buf.WriteString(header)
	buf.Write(data)
	if len(data)%2 != 0 {
		buf.WriteByte('\n')
	}
}

func createTarGz(filename, content string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	if filename != "" && content != "" {
		tw.WriteHeader(&tar.Header{Name: filename, Mode: 0644, Size: int64(len(content))})
		tw.Write([]byte(content))
	}

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func debSourceDsc(name, version string) []byte {
	dscContent := fmt.Sprintf(`Format: 3.0 (quilt)
Source: %s
Binary: %s
Architecture: any
Version: %s
Maintainer: Test Maintainer <test@example.com>
Homepage: https://example.com
Standards-Version: 4.6.0
Build-Depends: debhelper (>= 13)
Files:
 5d41402abc4b2a76b9719d911017c592 8388608 %s_%s.orig.tar.gz
 aaf4c61ddcc5e8a2dabede0f3b482cd9 524288 %s_%s.debian.tar.gz
`, name, name, version, name, "2.4.52", name, version)
	return []byte(dscContent)
}

func debSourceTarGz(name, version string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := fmt.Sprintf("Mock source for %s version %s\n", name, version)
	writetar(tw, fmt.Sprintf("%s-%s/README", name, version), content)

	tw.Close()
	gw.Close()
	return buf.Bytes()
}
