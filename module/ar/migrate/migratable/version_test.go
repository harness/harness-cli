package migratable

import (
	"context"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/rs/zerolog"
)

// fakeDestAdapterForCaseTest embeds noopAdapter and overrides GetAllFilesForVersion
// to return a mixed-case filename, proving the existingFileMap insert normalizes case.
type fakeDestAdapterForCaseTest struct {
	noopAdapter
}

func (f *fakeDestAdapterForCaseTest) GetAllFilesForVersion(ctx context.Context, registry, pkg, version string) ([]string, error) {
	return []string{"Company.Grpc.Pkg.1.0.0.nupkg"}, nil
}

// TestExistingFileMapInsertNormalizesCase verifies that the existingFileMap
// insert (version.go:117-119) lowercases the key, matching the lowercased lookup
// at version.go:180-181, so already-migrated files with mixed-case names are
// correctly recognized as existing.
func TestExistingFileMapInsertNormalizesCase(t *testing.T) {
	destAdapter := &fakeDestAdapterForCaseTest{}

	// Construct a Version directly (in-package test can access unexported fields)
	v := &Version{
		destAdapter:     destAdapter,
		existingFileMap: make(map[string]bool),
		logger:          zerolog.Nop(),
		pkg:             types.Package{Name: "company.grpc.pkg"},
		version:         types.Version{Name: "1.0.0"},
		artifactType:    types.NUGET,
		registry:        types.RegistryInfo{},
		config:          &types.Config{DryRun: false, Overwrite: false},
	}

	// Run Pre to populate existingFileMap
	if err := v.Pre(context.Background()); err != nil {
		t.Fatalf("Pre() failed: %v", err)
	}

	// Assert the lowercased key is present
	if !v.existingFileMap["company.grpc.pkg.1.0.0.nupkg"] {
		t.Errorf("existingFileMap missing lowercased key 'company.grpc.pkg.1.0.0.nupkg'")
	}

	// Assert the verbatim mixed-case key is absent (proof of normalization)
	if v.existingFileMap["Company.Grpc.Pkg.1.0.0.nupkg"] {
		t.Errorf("existingFileMap should NOT contain verbatim mixed-case key 'Company.Grpc.Pkg.1.0.0.nupkg'")
	}
}
