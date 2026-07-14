package harness

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// scopedGetRegistry resolves a destination registry identifier within the
// configured e2e scope (account[/org[/project]]). Used by the in-process migration
// fallback (E2E_IN_PROCESS=1); production getRegistry uses the same scoped lookup.
func scopedGetRegistry(t *testing.T, creds Creds, identifier string) (types.RegistryInfo, error) {
	t.Helper()

	c := newARClient(t, creds)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	registryRef := creds.registryRef(identifier)

	resp, err := c.GetRegistryWithResponse(ctx, registryRef)
	if err != nil {
		return types.RegistryInfo{}, fmt.Errorf("get registry %q: %w", identifier, err)
	}
	if resp.StatusCode() == http.StatusOK && resp.JSON200 != nil {
		reg := resp.JSON200.Data
		regType := ""
		if reg.Config != nil {
			regType = string(reg.Config.Type)
		}
		return types.RegistryInfo{
			Type: regType,
			URL:  reg.Url,
			Path: registryRef,
		}, nil
	}
	if resp.StatusCode() != http.StatusNotFound {
		return types.RegistryInfo{}, fmt.Errorf("get registry %q: status %d: %s",
			identifier, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}

	return findRegistryInSpace(ctx, c, creds.spaceRef(), identifier)
}

func findRegistryInSpace(
	ctx context.Context,
	c *ar.ClientWithResponses,
	spaceRef, identifier string,
) (types.RegistryInfo, error) {
	page := int64(0)
	size := int64(100)
	none := ar.GetAllRegistriesParamsScopeNone

	for {
		resp, err := c.GetAllRegistriesWithResponse(ctx, spaceRef, &ar.GetAllRegistriesParams{
			Page:       &page,
			Size:       &size,
			SearchTerm: &identifier,
			Scope:      &none,
		})
		if err != nil {
			return types.RegistryInfo{}, fmt.Errorf("list registries in %q: %w", spaceRef, err)
		}
		if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return types.RegistryInfo{}, fmt.Errorf("list registries in %q: status %d: %s",
				spaceRef, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}

		data := resp.JSON200.Data
		for _, v := range data.Registries {
			if v.Identifier != identifier {
				continue
			}
			return types.RegistryInfo{
				Type: string(v.Type),
				URL:  v.Url,
				Path: registryRefFromMetadata(spaceRef, identifier, v.Path),
			}, nil
		}

		if len(data.Registries) < int(size) ||
			(data.PageCount != nil && data.PageIndex != nil && (*data.PageIndex+1 >= *data.PageCount)) {
			break
		}
		page++
	}

	return types.RegistryInfo{}, fmt.Errorf("registry %q not found in space %q", identifier, spaceRef)
}

func registryRefFromMetadata(spaceRef, identifier string, path *string) string {
	if path != nil && *path != "" {
		return *path
	}
	return spaceRef + "/" + identifier
}
