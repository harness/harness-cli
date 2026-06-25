package nexus

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// helmSearchServer spins up an httptest server that answers Nexus' component
// search endpoint with the supplied response. It returns the server and the
// number of times the search endpoint was hit (to confirm pagination behavior).
func helmSearchServer(t *testing.T, resp NexusSearchResponse) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/service/rest/v1/search" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// newHelmAdapter builds a nexus adapter pointed at the test server.
func newHelmAdapter(t *testing.T, endpoint string) *adapter {
	t.Helper()
	a, err := newAdapter(types.RegistryConfig{Type: types.NEXUS, Endpoint: endpoint})
	if err != nil {
		t.Fatalf("newAdapter: %v", err)
	}
	return a.(*adapter)
}

// helmComponentFixture is a single helm component carrying a chart archive and
// its provenance sidecar, both tagged format "helm" (the .prov tag is
// environment-dependent in real Nexus, so we assert the path-based filter, not
// the tag).
func helmComponentFixture() NexusComponent {
	return NexusComponent{
		ID:         "c1",
		Repository: "helm-hosted",
		Format:     "helm",
		Name:       "nginx",
		Version:    "1.0.0",
		Assets: []NexusAsset{
			{Path: "nginx-1.0.0.tgz", Format: "helm", FileSize: 2048},
			{Path: "nginx-1.0.0.tgz.prov", Format: "helm", FileSize: 128},
		},
	}
}

// TestGetPackagesHelmHTTPNexus asserts that HELM_HTTP enumeration returns the
// chart archive as a package and excludes the .prov sidecar (migrated as a
// sibling by migrateHelmHTTPProv, never as its own package).
func TestGetPackagesHelmHTTPNexus(t *testing.T) {
	const registry = "helm-hosted"
	srv, hits := helmSearchServer(t, NexusSearchResponse{
		Items: []NexusComponent{helmComponentFixture()},
	})
	a := newHelmAdapter(t, srv.URL)

	pkgs, err := a.GetPackages(registry, types.HELM_HTTP, nil)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}

	if len(pkgs) != 1 {
		t.Fatalf("expected exactly 1 chart package, got %d: %+v", len(pkgs), pkgs)
	}
	p := pkgs[0]
	if p.Name != "nginx" {
		t.Errorf("package name = %q, want %q", p.Name, "nginx")
	}
	if p.Version != "1.0.0" {
		t.Errorf("package version = %q, want %q", p.Version, "1.0.0")
	}
	if p.URL != "nginx-1.0.0.tgz" {
		t.Errorf("package URL = %q, want chart .tgz path", p.URL)
	}
	if p.Registry != registry {
		t.Errorf("package registry = %q, want %q", p.Registry, registry)
	}

	// The provenance sidecar must not be enumerated as a package.
	for _, pkg := range pkgs {
		if pkg.URL == "nginx-1.0.0.tgz.prov" {
			t.Errorf("provenance sidecar was enumerated as a package: %+v", pkg)
		}
	}

	if *hits != 1 {
		t.Errorf("search endpoint hit %d times, want 1", *hits)
	}
}

// TestGetPackagesHelmHTTPNexusSkipsNonHelm asserts that non-helm assets in a
// component are skipped under HELM_HTTP enumeration even if a stray .tgz is
// present, and that a component contributing no chart asset yields no package.
func TestGetPackagesHelmHTTPNexusSkipsNonHelm(t *testing.T) {
	const registry = "helm-hosted"
	srv, _ := helmSearchServer(t, NexusSearchResponse{
		Items: []NexusComponent{
			{
				ID:      "c2",
				Format:  "raw",
				Name:    "not-a-chart",
				Version: "9.9.9",
				Assets: []NexusAsset{
					// Tagged non-helm: must be skipped regardless of suffix.
					{Path: "not-a-chart-9.9.9.tgz", Format: "raw", FileSize: 10},
				},
			},
		},
	})
	a := newHelmAdapter(t, srv.URL)

	pkgs, err := a.GetPackages(registry, types.HELM_HTTP, nil)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages for non-helm assets, got %d: %+v", len(pkgs), pkgs)
	}
}

// TestGetPackagesHelmLegacyNexusUnfiltered confirms the contrast: HELM_LEGACY
// enumeration is unfiltered, so the same fixture yields BOTH the chart and the
// .prov as packages (the path-based chart filter is HELM_HTTP-only).
func TestGetPackagesHelmLegacyNexusUnfiltered(t *testing.T) {
	const registry = "helm-hosted"
	srv, _ := helmSearchServer(t, NexusSearchResponse{
		Items: []NexusComponent{helmComponentFixture()},
	})
	a := newHelmAdapter(t, srv.URL)

	pkgs, err := a.GetPackages(registry, types.HELM_LEGACY, nil)
	if err != nil {
		t.Fatalf("GetPackages: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages (chart + prov) under HELM_LEGACY, got %d: %+v", len(pkgs), pkgs)
	}
	urls := map[string]bool{}
	for _, p := range pkgs {
		urls[p.URL] = true
	}
	if !urls["nginx-1.0.0.tgz"] || !urls["nginx-1.0.0.tgz.prov"] {
		t.Errorf("HELM_LEGACY should include both chart and prov, got URLs: %v", urls)
	}
}
