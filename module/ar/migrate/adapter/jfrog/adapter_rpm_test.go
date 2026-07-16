package jfrog

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// TestExtractRPMPackagesUsesPrimaryXMLPackageSize mirrors rpm-local mock data:
// primary.xml carries <size package="0"/> while the file tree fixture lists the
// RPM leaf at 1024 bytes (testdata/files/rpm-local.json). Enumeration currently
// trusts only primary.xml, so pkg.Size stays 0 and migrateRPM later logs 0.00B.
func TestExtractRPMPackagesUsesPrimaryXMLPackageSize(t *testing.T) {
	t.Helper()

	const primaryXML = `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" packages="1">
  <package type="rpm">
    <name>mockpkg</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.0.0" rel="1"/>
    <location href="mockpkg-1.0.0-1.x86_64.rpm"/>
    <size package="0"/>
  </package>
</metadata>`

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(primaryXML)); err != nil {
		t.Fatalf("write gzip payload: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	pkgs, err := extractRPMPackages(bytes.NewReader(buf.Bytes()), "rpm-local")
	if err != nil {
		t.Fatalf("extractRPMPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d: %+v", len(pkgs), pkgs)
	}

	got := pkgs[0]
	if got.Name != "mockpkg-1.0.0-1.x86_64.rpm" {
		t.Errorf("Name = %q, want filename from location href", got.Name)
	}
	if got.URL != "mockpkg-1.0.0-1.x86_64.rpm" {
		t.Errorf("URL = %q, want mockpkg-1.0.0-1.x86_64.rpm", got.URL)
	}

	// File-tree size for the same RPM in the mock is 1024; migration should not
	// report zero when the artifact has non-zero bytes.
	const fileTreeSize = 1024
	if got.Size != fileTreeSize {
		t.Errorf("Size = %d, want %d (non-zero size from repodata or file tree, not <size package=\"0\"/>)", got.Size, fileTreeSize)
	}
}
