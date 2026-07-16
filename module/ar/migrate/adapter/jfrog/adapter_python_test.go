package jfrog

import (
	"fmt"
	"strings"
	"testing"
)

func TestExtractPythonPackageNamesPreservesTrailingSlash(t *testing.T) {
	t.Helper()

	// JFrog/PyPI simple indexes commonly use directory hrefs like "requests/".
	html := `<html><body><a href="requests/">requests</a><br/></body></html>`
	names, err := extractPythonPackageNames(strings.NewReader(html))
	if err != nil {
		t.Fatalf("extractPythonPackageNames: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("expected 1 package href, got %d: %v", len(names), names)
	}

	// Bug: href is taken raw; trailing slash is not trimmed.
	if names[0] != "requests/" {
		t.Fatalf("href = %q, want %q (documents mock python-local simple.html)", names[0], "requests/")
	}

	// Intended package name for index lookups.
	if got, want := strings.TrimSuffix(names[0], "/"), "requests"; got != want {
		t.Errorf("trimmed name = %q, want %q", got, want)
	}
}

func TestPythonIndexPathDoubleSlashWithTrailingSlashPackageName(t *testing.T) {
	t.Helper()

	pkg := "requests/" // as returned by extractPythonPackageNames today
	indexPath := fmt.Sprintf(".pypi/%s/%s.html", pkg, pkg)
	if indexPath != ".pypi/requests//requests/.html" {
		t.Fatalf("indexPath = %q, want %q", indexPath, ".pypi/requests//requests/.html")
	}

	// Canonical on-disk path in python-local mock (single slash, .html suffix on name).
	const canonical = ".pypi/requests/requests.html"
	if indexPath == canonical {
		t.Fatal("broken index path must not equal canonical fixture path")
	}
}
