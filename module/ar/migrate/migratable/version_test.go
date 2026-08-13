package migratable

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// indexFakeSrc serves file bytes keyed by URI for the Version.Migrate file loop.
// A URI absent from content produces a download error (so an unexpected upload
// attempt surfaces loudly).
type indexFakeSrc struct {
	noopAdapter
	content map[string][]byte
}

func (s *indexFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, io.ErrUnexpectedEOF
	}
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

// indexFakeDest records the f.Name of every file it is asked to upload.
type indexFakeDest struct {
	noopAdapter
	uploaded []string
}

func (d *indexFakeDest) UploadFile(
	_ string,
	file io.ReadCloser,
	f *types.File,
	_ http.Header,
	_ string,
	_ string,
	_ types.ArtifactType,
	_ map[string]interface{},
) error {
	if file != nil {
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
	}
	d.uploaded = append(d.uploaded, f.Name)
	return nil
}

// genericFileTree builds a flat tree of leaf file nodes (one per name) so
// Version.Migrate's tree.GetAllFiles walk yields exactly those files.
func genericFileTree(names ...string) *types.TreeNode {
	root := &types.TreeNode{Name: "root", Key: "/", IsLeaf: false}
	for _, name := range names {
		f := &types.File{Name: name, Uri: "/" + name, Size: len(name)}
		root.Children = append(root.Children, types.TreeNode{
			Name:   name,
			Key:    "/" + name,
			IsLeaf: true,
			File:   f,
		})
	}
	return root
}

func newVersionJobForIndexTest(src, dest adp.Adapter, node *types.TreeNode, stats *types.TransferStats, idx *types.ExistingIndex) *Version {
	return &Version{
		srcRegistry:   "src-reg",
		destRegistry:  "dst-reg",
		srcAdapter:    src,
		destAdapter:   dest,
		artifactType:  types.GENERIC,
		logger:        zerolog.Nop(),
		pkg:           types.Package{Name: "my-package"},
		version:       types.Version{Name: "1.0.0"},
		node:          node,
		stats:         stats,
		config:        &types.Config{Concurrency: 1, DryRun: false, Overwrite: false},
		existingIndex: idx,
	}
}

// TestVersionMigrateSkipsFilesInIndex verifies that Version.Migrate reads the
// shared existingIndex directly: a file already present in the index is skipped
// (StatusSkip, no upload), while a file absent from the index is uploaded.
func TestVersionMigrateSkipsFilesInIndex(t *testing.T) {
	idx := types.NewExistingIndex()
	// The index is keyed by source-relative path (file.Uri), matching what
	// buildExistingIndex stores after harPathToSourcePath conversion.
	idx.AddFile("my-package", "1.0.0", "/already-there.txt")

	src := &indexFakeSrc{content: map[string][]byte{
		"/already-there.txt": []byte("existing"),
		"/new-file.txt":      []byte("fresh"),
	}}
	dest := &indexFakeDest{}
	stats := &types.TransferStats{}

	node := genericFileTree("already-there.txt", "new-file.txt")
	job := newVersionJobForIndexTest(src, dest, node, stats, idx)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Only the new file should have been uploaded.
	if len(dest.uploaded) != 1 || dest.uploaded[0] != "new-file.txt" {
		t.Errorf("dest uploads = %v, want [new-file.txt]", dest.uploaded)
	}

	// The already-present file should be recorded as a skip.
	var skipped, uploaded int
	for _, s := range stats.FileStats {
		switch s.Status {
		case types.StatusSkip:
			skipped++
			if s.Name != "already-there.txt" {
				t.Errorf("skip stat for unexpected file %q", s.Name)
			}
		case types.StatusSuccess:
			uploaded++
		}
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

// TestVersionMigrateIndexSkipIsCaseInsensitive is the AH-4458 regression guard:
// an already-migrated file whose destination name differs only in case from the
// source name must still be recognized as existing and skipped. HasFile
// lowercases both sides, so a mixed-case index entry matches a lower-case source
// file name (and vice versa).
func TestVersionMigrateIndexSkipIsCaseInsensitive(t *testing.T) {
	idx := types.NewExistingIndex()
	// Destination recorded the file with mixed case (keyed by source-relative
	// path, as buildExistingIndex stores it after harPathToSourcePath).
	idx.AddFile("my-package", "1.0.0", "/Company.Grpc.Pkg.1.0.0.nupkg")

	src := &indexFakeSrc{content: map[string][]byte{
		"/company.grpc.pkg.1.0.0.nupkg": []byte("pkg"),
	}}
	dest := &indexFakeDest{}
	stats := &types.TransferStats{}

	// Source enumerates the file in lower case.
	node := genericFileTree("company.grpc.pkg.1.0.0.nupkg")
	job := newVersionJobForIndexTest(src, dest, node, stats, idx)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	if len(dest.uploaded) != 0 {
		t.Errorf("case-insensitive match failed: unexpectedly uploaded %v", dest.uploaded)
	}
	if len(stats.FileStats) != 1 || stats.FileStats[0].Status != types.StatusSkip {
		t.Errorf("expected 1 StatusSkip stat, got %+v", stats.FileStats)
	}
}

// TestVersionMigrateNilIndexUploadsAll verifies that when no index is present
// (overwrite=true, or a non-indexable type, or index-build failure), Migrate
// performs no client-side skip and uploads every file. A non-nil index is the
// sole gate; there is no per-version destination lookup anymore.
func TestVersionMigrateNilIndexUploadsAll(t *testing.T) {
	src := &indexFakeSrc{content: map[string][]byte{
		"/a.txt": []byte("a"),
		"/b.txt": []byte("b"),
	}}
	dest := &indexFakeDest{}
	stats := &types.TransferStats{}

	node := genericFileTree("a.txt", "b.txt")
	job := newVersionJobForIndexTest(src, dest, node, stats, nil)

	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	if len(dest.uploaded) != 2 {
		t.Errorf("dest uploads = %v, want both files uploaded", dest.uploaded)
	}
}

// terraformFileTree builds a tree containing the given URI-keyed files, where
// each node's Name is the basename of the URI.
func terraformFileTree(uris ...string) *types.TreeNode {
	root := &types.TreeNode{Name: "root", Key: "/", IsLeaf: false}
	for _, uri := range uris {
		parts := strings.Split(strings.TrimPrefix(uri, "/"), "/")
		name := parts[len(parts)-1]
		f := &types.File{Name: name, Uri: uri, Size: len(name)}
		root.Children = append(root.Children, types.TreeNode{
			Name:   name,
			Key:    uri,
			IsLeaf: true,
			File:   f,
		})
	}
	return root
}

func newTerraformVersionJob(src, dest adp.Adapter, node *types.TreeNode, pkg, version string, stats *types.TransferStats) *Version {
	return &Version{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.TERRAFORM,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: pkg},
		version:      types.Version{Name: version},
		node:         node,
		stats:        stats,
		config:       &types.Config{Concurrency: 1, DryRun: false},
		mapping:      &types.RegistryMapping{},
	}
}

// TestVersionMigrateTerraformModuleFilter verifies that the TERRAFORM branch in
// Version.Migrate only enqueues files whose path matches the current pkg+version.
func TestVersionMigrateTerraformModuleFilter(t *testing.T) {
	// tree contains two versions of the same module; only 1.0.0 should be uploaded.
	node := terraformFileTree(
		"/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz",
		"/hashicorp/vpc/aws/1.1.0/vpc-1.1.0.tar.gz",
	)
	src := &indexFakeSrc{content: map[string][]byte{
		"/hashicorp/vpc/aws/1.0.0/vpc-1.0.0.tar.gz": []byte("module-bytes"),
		"/hashicorp/vpc/aws/1.1.0/vpc-1.1.0.tar.gz": []byte("module-bytes-v2"),
	}}
	dest := &indexFakeDest{}
	stats := &types.TransferStats{}

	job := newTerraformVersionJob(src, dest, node, "hashicorp/vpc/aws", "1.0.0", stats)
	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	if len(dest.uploaded) != 1 || dest.uploaded[0] != "vpc-1.0.0.tar.gz" {
		t.Errorf("uploaded = %v, want [vpc-1.0.0.tar.gz]", dest.uploaded)
	}
}

// TestVersionMigrateTerraformProviderFilter verifies provider files are filtered
// by pkg (ns/type) and version, and files for other versions are skipped.
func TestVersionMigrateTerraformProviderFilter(t *testing.T) {
	node := terraformFileTree(
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip",
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_darwin_arm64.zip",
		"/hashicorp/aws/1.0.0/terraform-provider-aws_1.0.0_linux_amd64.zip",
	)
	src := &indexFakeSrc{content: map[string][]byte{
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip":  []byte("p1"),
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_darwin_arm64.zip": []byte("p2"),
		"/hashicorp/aws/1.0.0/terraform-provider-aws_1.0.0_linux_amd64.zip":  []byte("p3"),
	}}
	dest := &indexFakeDest{}
	stats := &types.TransferStats{}

	job := newTerraformVersionJob(src, dest, node, "hashicorp/aws", "2.0.0", stats)
	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	if len(dest.uploaded) != 2 {
		t.Errorf("uploaded = %v, want 2 files for version 2.0.0", dest.uploaded)
	}
	for _, name := range dest.uploaded {
		if !strings.Contains(name, "2.0.0") {
			t.Errorf("uploaded unexpected file %q (not version 2.0.0)", name)
		}
	}
}

// TestVersionMigrateTerraformUnrecognisedPathSkipped verifies that files with
// an unrecognised path shape (neither module nor provider) are silently skipped.
func TestVersionMigrateTerraformUnrecognisedPathSkipped(t *testing.T) {
	node := terraformFileTree(
		"/hashicorp/aws/2.0.0/something-unexpected.zip",
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip",
	)
	src := &indexFakeSrc{content: map[string][]byte{
		"/hashicorp/aws/2.0.0/terraform-provider-aws_2.0.0_linux_amd64.zip": []byte("p1"),
	}}
	dest := &indexFakeDest{}
	stats := &types.TransferStats{}

	job := newTerraformVersionJob(src, dest, node, "hashicorp/aws", "2.0.0", stats)
	if err := job.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Only the valid provider file should be uploaded; the unrecognised .zip is dropped.
	if len(dest.uploaded) != 1 || dest.uploaded[0] != "terraform-provider-aws_2.0.0_linux_amd64.zip" {
		t.Errorf("uploaded = %v, want only the valid provider file", dest.uploaded)
	}
}
