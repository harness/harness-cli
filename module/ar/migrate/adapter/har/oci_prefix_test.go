package har

import (
	"testing"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

func newTestAdapter() *adapter {
	return &adapter{
		reg:              types.RegistryConfig{Endpoint: "https://ancestry.harness.io"},
		registryURLCache: make(map[string]string),
	}
}

// --- ociPrefixFromCache: URL cached by GetRegistry ---

func TestOCIPrefixFromCache_VanityURL(t *testing.T) {
	a := newTestAdapter()
	a.registryURLCache["helmoci"] = "https://ancestry.harness.io/oci/helmoci"

	if got := a.ociPrefixFromCache("helmoci"); got != "oci" {
		t.Fatalf("expected 'oci', got %q", got)
	}
}

func TestOCIPrefixFromCache_AccountIDURL(t *testing.T) {
	a := newTestAdapter()
	a.registryURLCache["helmoci"] = "https://ancestry.harness.io/cetpgmqtq22qdnkymdp_9a/helmoci"

	if got := a.ociPrefixFromCache("helmoci"); got != "cetpgmqtq22qdnkymdp_9a" {
		t.Fatalf("expected accountID prefix, got %q", got)
	}
}

func TestOCIPrefixFromCache_NoCacheReturnsFallback(t *testing.T) {
	orig := config.Global
	config.Global.AccountID = "MyAccID"
	defer func() { config.Global = orig }()

	a := newTestAdapter()
	// nothing in cache → falls back to lowercased accountID
	if got := a.ociPrefixFromCache("helmoci"); got != "myaccid" {
		t.Fatalf("expected 'myaccid', got %q", got)
	}
}

func TestOCIPrefixFromCache_MultiSegmentPrefix(t *testing.T) {
	// Defensive: registry URL with no standard prefix — just use whatever is there.
	a := newTestAdapter()
	a.registryURLCache["myreg"] = "https://example.harness.io/a/b/myreg"

	// path = /a/b/myreg → strip /myreg → /a/b → trim → "a/b"
	if got := a.ociPrefixFromCache("myreg"); got != "a/b" {
		t.Fatalf("expected 'a/b', got %q", got)
	}
}

func TestOCIPrefixFromCache_InvalidURLFallsBack(t *testing.T) {
	orig := config.Global
	config.Global.AccountID = "AccXYZ"
	defer func() { config.Global = orig }()

	a := newTestAdapter()
	a.registryURLCache["helmoci"] = "://not-a-valid-url"

	if got := a.ociPrefixFromCache("helmoci"); got != "accxyz" {
		t.Fatalf("expected fallback 'accxyz', got %q", got)
	}
}
