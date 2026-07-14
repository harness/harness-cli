package harness

import (
	"context"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// scopedDestAdapter wraps a HAR destination adapter and overrides GetRegistry
// with the e2e scoped lookup so migration pre-step does not scan the whole QA
// account. All other adapter methods delegate to the embedded implementation.
type scopedDestAdapter struct {
	adapter.Adapter
	creds Creds
	t     *testing.T
}

func newScopedDestAdapter(t *testing.T, inner adapter.Adapter, creds Creds) adapter.Adapter {
	return &scopedDestAdapter{Adapter: inner, creds: creds, t: t}
}

func (s *scopedDestAdapter) GetRegistry(ctx context.Context, registry string) (types.RegistryInfo, error) {
	return scopedGetRegistry(s.t, s.creds, registry)
}
