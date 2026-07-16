package mock_jfrog

import (
	"context"
	"strings"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog/log"
)

func init() {
	adapterType := types.MOCK_JFROG
	if err := adp.RegisterFactory(adapterType, new(factory)); err != nil {
		return
	}
}

type factory struct{}

func (f factory) Create(ctx context.Context, config types.RegistryConfig) (adp.Adapter, error) {
	inner := jfrog.NewAdapterWithClient(config, NewMockClient())
	return &mockAdapter{Adapter: inner}, nil
}

// mockAdapter wraps the JFrog adapter with test-fixture overrides that belong in
// the mock layer, not production jfrog/adapter.go.
type mockAdapter struct {
	adp.Adapter
}

func (m *mockAdapter) GetPackages(registry string, artifactType types.ArtifactType, root *types.TreeNode) (
	packages []types.Package,
	err error,
) {
	// composer-logical-local is a ticket-#18 repro fixture: the file tree holds
	// multiple version zips for one logical package, but enumeration returns a
	// single Package row (as if grouped by vendor/package). GetVersions does not
	// scan for sibling versions, so only the first zip is migrated.
	if artifactType == types.COMPOSER && registry == "composer-logical-local" {
		leaves, _ := tree.GetAllFiles(root)
		var first *types.File
		zipCount := 0
		for _, leaf := range leaves {
			if leaf.Folder || !strings.HasSuffix(leaf.Uri, ".zip") {
				continue
			}
			zipCount++
			if first == nil {
				first = leaf
			}
		}
		if first != nil {
			packages = append(packages, types.Package{
				Registry: registry,
				Path:     "/",
				Name:     "vendor/package",
				Size:     first.Size,
				URL:      first.Uri,
			})
			log.Info().Msgf("composer-logical-local: %d zip(s) in tree, enumerated 1 logical package %q",
				zipCount, "vendor/package")
		}
		return packages, nil
	}

	return m.Adapter.GetPackages(registry, artifactType, root)
}
