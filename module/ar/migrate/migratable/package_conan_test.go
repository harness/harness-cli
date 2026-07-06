package migratable

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/rs/zerolog"
)

// conanUpload records the coordinates of a single UploadFile call so ordering
// and layer routing can be asserted.
type conanUpload struct {
	name  string
	layer string
	rrev  string
	pkgid string
	prev  string
	sha1  string
}

// conanFakeSrc serves Conan file bytes keyed by Uri; an unknown Uri errors.
type conanFakeSrc struct {
	noopAdapter
	content map[string][]byte
}

func (s *conanFakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

// conanFakeDest records each upload's coordinates (from metadata) and can be
// told to fail or skip (ErrArtifactAlreadyExists) a named file.
type conanFakeDest struct {
	noopAdapter
	uploads  []conanUpload
	failName string
	skipName string
}

func (d *conanFakeDest) UploadFile(
	_ string,
	file io.ReadCloser,
	_ *types.File,
	_ http.Header,
	_ string,
	_ string,
	_ types.ArtifactType,
	metadata map[string]interface{},
) error {
	// The real adapters own the reader; drain+close so the source is consumed.
	if file != nil {
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
	}
	get := func(k string) string {
		v, _ := metadata[k].(string)
		return v
	}
	name := get("filename")
	if d.skipName != "" && name == d.skipName {
		return types.ErrArtifactAlreadyExists
	}
	if d.failName != "" && name == d.failName {
		return fmt.Errorf("upload %q failed", name)
	}
	d.uploads = append(d.uploads, conanUpload{
		name:  name,
		layer: get("layer"),
		rrev:  get("rrev"),
		pkgid: get("pkgid"),
		prev:  get("prev"),
		sha1:  get("sha1"),
	})
	return nil
}

const (
	conanRRev  = "9a0b1c2d3e4f5061728394a5b6c7d8e9"
	conanPkgID = "abcabcabcabcabcabcabcabcabcabcabcabcabca"
	conanPRev  = "1f2e3d4c5b6a7988990a1b2c3d4e5f60"
	conanRef   = "/zlib/1.2.13/_/_"
)

// conanFixtureFiles returns a full single-reference layout: recipe layer (3
// files) plus one package layer (3 files), each with a distinct SHA1.
func conanFixtureFiles() []types.File {
	base := conanRef + "/" + conanRRev
	pkg := base + "/package/" + conanPkgID + "/" + conanPRev
	return []types.File{
		{Uri: base + "/export/conanfile.py", SHA1: "sha-recipe-py", Size: 10},
		{Uri: base + "/export/conan_export.tgz", SHA1: "sha-recipe-tgz", Size: 20},
		{Uri: base + "/export/conanmanifest.txt", SHA1: "sha-recipe-man", Size: 30},
		{Uri: pkg + "/conaninfo.txt", SHA1: "sha-pkg-info", Size: 40},
		{Uri: pkg + "/conan_package.tgz", SHA1: "sha-pkg-tgz", Size: 50},
		{Uri: pkg + "/conanmanifest.txt", SHA1: "sha-pkg-man", Size: 60},
	}
}

func conanContent(files []types.File) map[string][]byte {
	m := make(map[string][]byte, len(files))
	for _, f := range files {
		m[f.Uri] = []byte("bytes:" + f.Uri)
	}
	return m
}

func newConanJob(src, dest adp.Adapter, stats *types.TransferStats) (*Package, error) {
	files := conanFixtureFiles()
	root := tree.TransformToTree(files)
	node, err := tree.GetNodeForPath(root, conanRef)
	if err != nil {
		return nil, err
	}
	return &Package{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.CONAN,
		logger:       zerolog.Nop(),
		pkg:          types.Package{Name: "zlib/1.2.13", Path: conanRef},
		node:         node,
		stats:        stats,
		config:       &types.Config{},
	}, nil
}

// TestMigrateConanHappyPath: all six files upload successfully. Recipe layer is
// uploaded before the package layer, conanmanifest.txt is last within each
// layer, coordinates route to the correct layer, and the source SHA1 is
// forwarded on every upload.
func TestMigrateConanHappyPath(t *testing.T) {
	files := conanFixtureFiles()
	src := &conanFakeSrc{content: conanContent(files)}
	dest := &conanFakeDest{}
	stats := &types.TransferStats{}

	job, err := newConanJob(src, dest, stats)
	if err != nil {
		t.Fatalf("build job: %v", err)
	}
	if err := job.migrateConan(context.Background()); err != nil {
		t.Fatalf("migrateConan: %v", err)
	}

	if len(dest.uploads) != 6 {
		t.Fatalf("expected 6 uploads, got %d: %+v", len(dest.uploads), dest.uploads)
	}
	if len(stats.FileStats) != 6 {
		t.Fatalf("expected 6 FileStats, got %d", len(stats.FileStats))
	}
	for _, s := range stats.FileStats {
		if s.Status != types.StatusSuccess {
			t.Errorf("expected Success for %s, got %s (%s)", s.Name, s.Status, s.Error)
		}
	}

	// All recipe-layer uploads must come before any package-layer upload.
	seenPackage := false
	for i, u := range dest.uploads {
		switch u.layer {
		case "recipe":
			if seenPackage {
				t.Errorf("recipe upload %q at index %d appears after a package upload", u.name, i)
			}
			if u.rrev != conanRRev || u.pkgid != "" || u.prev != "" {
				t.Errorf("recipe upload %q has wrong coords: %+v", u.name, u)
			}
		case "package":
			seenPackage = true
			if u.rrev != conanRRev || u.pkgid != conanPkgID || u.prev != conanPRev {
				t.Errorf("package upload %q has wrong coords: %+v", u.name, u)
			}
		default:
			t.Errorf("upload %q has unexpected layer %q", u.name, u.layer)
		}
		if u.sha1 == "" {
			t.Errorf("upload %q missing forwarded SHA1", u.name)
		}
	}

	// conanmanifest.txt must be the last file within each layer group.
	lastRecipe, lastPackage := "", ""
	for _, u := range dest.uploads {
		if u.layer == "recipe" {
			lastRecipe = u.name
		} else {
			lastPackage = u.name
		}
	}
	if lastRecipe != "conanmanifest.txt" {
		t.Errorf("last recipe upload = %q, want conanmanifest.txt", lastRecipe)
	}
	if lastPackage != "conanmanifest.txt" {
		t.Errorf("last package upload = %q, want conanmanifest.txt", lastPackage)
	}
}

// TestMigrateConanSkipsExisting: a 409 (ErrArtifactAlreadyExists) from the
// destination is recorded as StatusSkip, not a failure, and does not abort the
// remaining uploads.
func TestMigrateConanSkipsExisting(t *testing.T) {
	files := conanFixtureFiles()
	src := &conanFakeSrc{content: conanContent(files)}
	dest := &conanFakeDest{skipName: "conanfile.py"}
	stats := &types.TransferStats{}

	job, err := newConanJob(src, dest, stats)
	if err != nil {
		t.Fatalf("build job: %v", err)
	}
	if err := job.migrateConan(context.Background()); err != nil {
		t.Fatalf("migrateConan: %v", err)
	}

	var skips, success, fails int
	for _, s := range stats.FileStats {
		switch s.Status {
		case types.StatusSkip:
			skips++
		case types.StatusSuccess:
			success++
		case types.StatusFail:
			fails++
		}
	}
	if skips != 1 || success != 5 || fails != 0 {
		t.Errorf("expected 1 skip / 5 success / 0 fail, got %d/%d/%d", skips, success, fails)
	}
	// The skipped file must not appear in the destination's recorded uploads.
	for _, u := range dest.uploads {
		if u.name == "conanfile.py" {
			t.Errorf("skipped file conanfile.py should not be recorded as uploaded")
		}
	}
}

// TestMigrateConanDryRun: dry-run performs no downloads/uploads and records no
// transfer FileStats.
func TestMigrateConanDryRun(t *testing.T) {
	files := conanFixtureFiles()
	src := &conanFakeSrc{content: conanContent(files)}
	dest := &conanFakeDest{}
	stats := &types.TransferStats{}

	job, err := newConanJob(src, dest, stats)
	if err != nil {
		t.Fatalf("build job: %v", err)
	}
	job.config = &types.Config{DryRun: true}

	if err := job.migrateConan(context.Background()); err != nil {
		t.Fatalf("migrateConan (dry-run): %v", err)
	}
	if len(dest.uploads) != 0 {
		t.Errorf("dry-run must not upload, got: %+v", dest.uploads)
	}
	if len(stats.FileStats) != 0 {
		t.Errorf("dry-run must not record FileStats, got: %+v", stats.FileStats)
	}
}
