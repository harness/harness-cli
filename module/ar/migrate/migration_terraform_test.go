package migrate

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

// TestRunTerraformProviderVersionIsAtomic verifies §10-W1: a TERRAFORM
// provider version is a network-mirror unit — ALL platform files of an
// in-scope version must migrate together. With a date filter that keeps only
// the linux/amd64 zip of hashicorp/aws 2.0.0 (the darwin/arm64 zip is out of
// window), the version is in scope (one file survived) and must migrate BOTH
// zips; publishing only the survivor would be a partial, broken mirror.
func TestRunTerraformProviderVersionIsAtomic(t *testing.T) {
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		Mappings: []types.RegistryMapping{{
			ArtifactType:        types.TERRAFORM,
			SourceRegistry:      "terraform-local",
			DestinationRegistry: "dst-reg",
			DateFilter:          &types.DateFilter{Match: types.DateFilterMatchAny, CreatedAfter: &after},
		}},
	}
	dest := &fakeDestAdapter{}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	uploads := dest.uploadedURIs()
	sort.Strings(uploads)
	want := []string{
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_darwin_arm64.zip",
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip",
	}
	if len(uploads) != len(want) {
		t.Fatalf("expected %d uploads (full provider version), got %d: %v", len(want), len(uploads), uploads)
	}
	for i := range want {
		if uploads[i] != want[i] {
			t.Fatalf("uploads mismatch:\n got: %v\nwant: %v", uploads, want)
		}
	}
}

// TestRunTerraformWhollyOutOfWindowVersionOmitted verifies the flip side of
// version atomicity: a version with NO files in the date window is omitted
// entirely (no partial upload, no empty version).
func TestRunTerraformWhollyOutOfWindowVersionOmitted(t *testing.T) {
	// Everything in the terraform-local fixture predates this cutoff.
	after := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		Mappings: []types.RegistryMapping{{
			ArtifactType:        types.TERRAFORM,
			SourceRegistry:      "terraform-local",
			DestinationRegistry: "dst-reg",
			DateFilter:          &types.DateFilter{Match: types.DateFilterMatchAny, CreatedAfter: &after},
		}},
	}
	dest := &fakeDestAdapter{}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error for fully out-of-window run, got: %v", err)
	}
	if uploads := dest.uploadedURIs(); len(uploads) != 0 {
		t.Fatalf("expected zero uploads when every file is out of window, got: %v", uploads)
	}
}
