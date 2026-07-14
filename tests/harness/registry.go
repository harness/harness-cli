package harness

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/util/common/auth"
)

// newARClient builds a management-API client for the HAR registries API using
// the same base path and x-api-key auth the migration uses.
func newARClient(t *testing.T, creds Creds) *ar.ClientWithResponses {
	t.Helper()
	ApplyGlobalConfig(creds) // auth editor reads config.Global.AuthToken
	c, err := ar.NewClientWithResponses(
		creds.APIURL+"/gateway/har/api/v1",
		ar.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
		auth.GetXApiKeyOptionAR(),
	)
	if err != nil {
		t.Fatalf("failed to create AR client: %v", err)
	}
	return c
}

// CreateRegistry provisions a VIRTUAL HAR registry of the given package type in
// the configured scope and returns its fully qualified registry reference for
// later deletion. An already-existing registry (from a retried run) is treated
// as success.
func CreateRegistry(t *testing.T, creds Creds, identifier, packageType string) string {
	t.Helper()

	c := newARClient(t, creds)

	var regCfg ar.RegistryConfig
	if err := regCfg.FromVirtualConfig(ar.VirtualConfig{}); err != nil {
		t.Fatalf("failed to build virtual registry config: %v", err)
	}

	spaceRef := creds.spaceRef()
	desc := "harness-cli e2e migration test registry"
	// parentRef (the space path) is required in the body: the server forms the
	// registry reference from it. Passing only the space_ref query param yields a
	// 400 "invalid registry reference".
	body := ar.RegistryRequest{
		Identifier:  identifier,
		PackageType: ar.PackageType(packageType),
		Config:      &regCfg,
		Description: &desc,
		ParentRef:   &spaceRef,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := c.CreateRegistryWithResponse(ctx, &ar.CreateRegistryParams{SpaceRef: &spaceRef}, body)
	if err != nil {
		t.Fatalf("failed to create registry %q: %v", identifier, err)
	}

	switch {
	case resp.StatusCode() == http.StatusCreated:
		t.Logf("created registry %q (packageType=%s)", identifier, packageType)
	case resp.StatusCode() == http.StatusConflict,
		(resp.StatusCode() == http.StatusBadRequest && strings.Contains(strings.ToLower(string(resp.Body)), "already")):
		t.Logf("registry %q already exists, reusing", identifier)
	default:
		t.Fatalf("failed to create registry %q: status %d: %s", identifier, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}

	return creds.registryRef(identifier)
}

// DeleteRegistry removes a registry created by CreateRegistry. Cleanup failures
// are logged but do not fail the test.
func DeleteRegistry(t *testing.T, creds Creds, ref string) {
	t.Helper()

	c := newARClient(t, creds)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := c.DeleteRegistryWithResponse(ctx, ref)
	if err != nil {
		t.Logf("cleanup: failed to delete registry %q: %v", ref, err)
		return
	}
	if resp.StatusCode() != http.StatusOK {
		t.Logf("cleanup: unexpected status %d deleting registry %q: %s", resp.StatusCode(), ref, strings.TrimSpace(string(resp.Body)))
		return
	}
	t.Logf("deleted registry %q", ref)
}
