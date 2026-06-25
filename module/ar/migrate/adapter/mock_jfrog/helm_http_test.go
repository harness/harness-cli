package mock_jfrog

import (
	"context"
	"io"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestGetPackagesHelmHTTP exercises the JFrog adapter's HELM_HTTP hybrid
// enumeration through the mock client: index.yaml (flat nginx) plus a tree
// sweep that recovers charts absent from the index (nested abc, flat orphan),
// deduped against the index by leaf file name.
func TestGetPackagesHelmHTTP(t *testing.T) {
	const registry = "helm-http-local"
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())

	files, err := adapter.GetFiles(registry)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	root := tree.TransformToTree(files)

	pkgs, err := adapter.GetPackages(registry, types.HELM_HTTP, root)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	// Expect exactly 5 charts: nginx (index), abc (tree, nested), orphan (tree),
	// and team-a/abc + team-b/abc — two DISTINCT nested charts that share a leaf
	// name+version and must both survive (no leaf-name collision). The .prov
	// sidecar and index.yaml must NOT be enumerated as packages.
	byName := make(map[string]types.Package)
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if len(pkgs) != 5 {
		t.Fatalf("expected 5 charts, got %d: %+v", len(pkgs), pkgs)
	}

	cases := []struct {
		name    string
		version string
	}{
		{"nginx", "1.0.0"},             // from index.yaml
		{"ChartA/ChartB/abc", "1.0.1"}, // from tree sweep, nested prefix preserved
		{"orphan", "2.0.0"},            // from tree sweep, flat
		{"team-a/abc", "1.0.1"},        // collision case — distinct nested identity
		{"team-b/abc", "1.0.1"},        // collision case — distinct nested identity
	}
	for _, c := range cases {
		p, ok := byName[c.name]
		if !ok {
			t.Errorf("missing chart %q in %+v", c.name, byName)
			continue
		}
		if p.Version != c.version {
			t.Errorf("chart %q version = %q, want %q", c.name, p.Version, c.version)
		}
	}

	// The .prov must not appear as its own package.
	for _, p := range pkgs {
		if p.Name == "nginx-1.0.0.tgz.prov" || p.URL == "/nginx-1.0.0.tgz.prov" {
			t.Errorf("provenance file was enumerated as a package: %+v", p)
		}
	}

	// The discovered chart bytes must be downloadable (valid gzip tar fixtures).
	for _, c := range cases {
		p := byName[c.name]
		rc, _, err := adapter.DownloadFile(registry, p.URL)
		if err != nil {
			t.Errorf("DownloadFile(%q): %v", p.URL, err)
			continue
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if len(b) == 0 {
			t.Errorf("DownloadFile(%q) returned empty bytes", p.URL)
		}
	}

	// The provenance sibling of the indexed chart must be downloadable at
	// "<chartURL>.prov" — the path migrateHelmHTTPProv uses.
	nginx := byName["nginx"]
	provRC, _, err := adapter.DownloadFile(registry, nginx.URL+".prov")
	if err != nil {
		t.Errorf("DownloadFile prov sibling: %v", err)
	} else {
		provRC.Close()
	}

	_ = context.Background()
}

// TestGetVersionsHelmHTTP confirms the adapter returns the single empty version
// (the direct package path does not fan out per-version for HELM_HTTP).
func TestGetVersionsHelmHTTP(t *testing.T) {
	adapter := jfrog.NewAdapterWithClient(types.RegistryConfig{Type: types.MOCK_JFROG}, NewMockClient())
	versions, err := adapter.GetVersions(types.Package{Name: "nginx"}, nil, "helm-http-local", "nginx", types.HELM_HTTP)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Name != "" {
		t.Fatalf("expected single empty version, got %+v", versions)
	}
}
