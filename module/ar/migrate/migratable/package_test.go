package migratable

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	adp "github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/rs/zerolog"
)

// noopAdapter implements adapter.Adapter with not-implemented stubs so concrete
// fakes only need to override the two methods migrateHelmHTTP exercises
// (DownloadFile, UploadFile). It keeps the test focused on the package-job flow.
type noopAdapter struct{}

func (noopAdapter) GetKeyChain(string) (authn.Keychain, error) { return nil, nil }
func (noopAdapter) GetConfig() types.RegistryConfig            { return types.RegistryConfig{} }
func (noopAdapter) ValidateCredentials() (bool, error)         { return true, nil }
func (noopAdapter) GetRegistry(context.Context, string) (types.RegistryInfo, error) {
	return types.RegistryInfo{}, nil
}
func (noopAdapter) CreateRegistryIfDoesntExist(string, types.ArtifactType) (bool, error) {
	return false, nil
}
func (noopAdapter) GetPackages(string, types.ArtifactType, *types.TreeNode) ([]types.Package, error) {
	return nil, nil
}
func (noopAdapter) GetVersions(types.Package, *types.TreeNode, string, string, types.ArtifactType) ([]types.Version, error) {
	return nil, nil
}
func (noopAdapter) GetFiles(string) ([]types.File, error) { return nil, nil }
func (noopAdapter) DownloadFile(string, string) (io.ReadCloser, http.Header, error) {
	return nil, nil, fmt.Errorf("not implemented")
}
func (noopAdapter) UploadFile(string, io.ReadCloser, *types.File, http.Header, string, string, types.ArtifactType, map[string]interface{}) error {
	return fmt.Errorf("not implemented")
}
func (noopAdapter) GetOCIImagePath(string, string, string) (string, error) { return "", nil }
func (noopAdapter) AddNPMTag(string, string, string, string) error         { return nil }
func (noopAdapter) VersionExists(context.Context, types.Package, string, string, string, types.ArtifactType) (bool, error) {
	return false, nil
}
func (noopAdapter) FileExists(context.Context, string, string, string, *types.File, types.ArtifactType) (bool, error) {
	return false, nil
}
func (noopAdapter) GetAllFilesForVersion(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (noopAdapter) CreateVersion(string, string, string, types.ArtifactType, []*types.PackageFiles, map[string]interface{}) error {
	return nil
}

// fakeSrc serves chart/prov bytes keyed by URI. A URI absent from content
// produces a download error (used to model a missing .prov or an unreachable
// chart).
type fakeSrc struct {
	noopAdapter
	content map[string][]byte // uri -> bytes
}

func (s *fakeSrc) DownloadFile(_ string, uri string) (io.ReadCloser, http.Header, error) {
	b, ok := s.content[uri]
	if !ok {
		return nil, nil, fmt.Errorf("download %q: not found", uri)
	}
	// Return an empty header on purpose (no Content-Length), mirroring the mock
	// adapter. The prov size must still be reported accurately from the bytes.
	return io.NopCloser(strings.NewReader(string(b))), http.Header{}, nil
}

// fakeDest records every upload it is handed and can be told to fail uploads
// whose file name ends with a configured suffix (used to model a chart-ok /
// prov-fail split, or a chart-upload failure).
type fakeDest struct {
	noopAdapter
	uploaded    []string // f.Name values, in order
	failSuffix  string   // if non-empty, UploadFile errors when f.Name ends with it
	drainReader bool
}

func (d *fakeDest) UploadFile(
	_ string,
	file io.ReadCloser,
	f *types.File,
	_ http.Header,
	_ string,
	_ string,
	_ types.ArtifactType,
	_ map[string]interface{},
) error {
	// UploadFile owns the reader; drain+close it like the real adapters do so
	// the source NopCloser is consumed.
	if file != nil {
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
	}
	if d.failSuffix != "" && strings.HasSuffix(f.Name, d.failSuffix) {
		return fmt.Errorf("upload %q failed", f.Name)
	}
	d.uploaded = append(d.uploaded, f.Name)
	return nil
}

// newHelmHTTPJob constructs a Package job wired with the given fakes. The job is
// built directly (not via NewPackageJob) so the test owns the unexported fields
// and uses a silent logger.
func newHelmHTTPJob(src adp.Adapter, dest adp.Adapter, pkg types.Package, stats *types.TransferStats) *Package {
	return &Package{
		srcRegistry:  "src-reg",
		destRegistry: "dst-reg",
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: types.HELM_HTTP,
		logger:       zerolog.Nop(),
		pkg:          pkg,
		stats:        stats,
		config:       &types.Config{},
	}
}

// TestMigrateHelmHTTPHappyPathWithProv: chart + prov both present and upload
// cleanly → two Success FileStats, and the dest receives the canonical
// "<name>-<version>.tgz" and "<name>-<version>.tgz.prov" names.
func TestMigrateHelmHTTPHappyPathWithProv(t *testing.T) {
	pkg := types.Package{Name: "nginx", Version: "1.0.0", URL: "/nginx-1.0.0.tgz", Size: 2048}
	src := &fakeSrc{content: map[string][]byte{
		"/nginx-1.0.0.tgz":      []byte("chart-bytes"),
		"/nginx-1.0.0.tgz.prov": []byte("prov-bytes"),
	}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)

	if err := job.migrateHelmHTTP(context.Background()); err != nil {
		t.Fatalf("migrateHelmHTTP returned err: %v", err)
	}

	if len(stats.FileStats) != 2 {
		t.Fatalf("expected 2 FileStats (chart + prov), got %d: %+v", len(stats.FileStats), stats.FileStats)
	}
	for _, s := range stats.FileStats {
		if s.Status != types.StatusSuccess {
			t.Errorf("expected Success, got %s (err=%q) for uri %s", s.Status, s.Error, s.Uri)
		}
	}
	// Chart stat reports the enumerated pkg.Size; the prov stat reports the
	// actual provenance byte count read from the source (regression guard: prov
	// size was hardcoded 0, and Content-Length is absent here on purpose).
	chartStat, provStat := stats.FileStats[0], stats.FileStats[1]
	if chartStat.Size != 2048 {
		t.Errorf("chart stat size = %d, want 2048 (pkg.Size)", chartStat.Size)
	}
	if provStat.Size != int64(len("prov-bytes")) {
		t.Errorf("prov stat size = %d, want %d (actual prov bytes)", provStat.Size, len("prov-bytes"))
	}
	wantUploads := []string{"nginx-1.0.0.tgz", "nginx-1.0.0.tgz.prov"}
	if strings.Join(dest.uploaded, ",") != strings.Join(wantUploads, ",") {
		t.Errorf("dest uploads = %v, want %v", dest.uploaded, wantUploads)
	}
}

// TestMigrateHelmHTTPDryRunSkipsTransfer: in dry-run mode migrateHelmHTTP must
// not download from the source or upload to the destination. Enumeration records
// the chart separately; the per-chart transfer is a no-op. Regression guard for
// the dry-run leak where HELM_HTTP performed real downloads/uploads.
func TestMigrateHelmHTTPDryRunSkipsTransfer(t *testing.T) {
	pkg := types.Package{Name: "nginx", Version: "1.0.0", URL: "/nginx-1.0.0.tgz", Size: 2048}
	src := &fakeSrc{content: map[string][]byte{
		"/nginx-1.0.0.tgz":      []byte("chart-bytes"),
		"/nginx-1.0.0.tgz.prov": []byte("prov-bytes"),
	}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)
	job.config = &types.Config{DryRun: true}

	if err := job.migrateHelmHTTP(context.Background()); err != nil {
		t.Fatalf("migrateHelmHTTP (dry-run) returned err: %v", err)
	}
	if len(dest.uploaded) != 0 {
		t.Errorf("dry-run must not upload, but dest received: %v", dest.uploaded)
	}
	if len(stats.FileStats) != 0 {
		t.Errorf("dry-run must not record transfer FileStats, got: %+v", stats.FileStats)
	}
}

// TestMigrateHelmHTTPChartOnly: chart present, no prov sibling → one Success
// FileStat and NO failure stat for the missing prov (missing prov is normal).
func TestMigrateHelmHTTPChartOnly(t *testing.T) {
	pkg := types.Package{Name: "nginx", Version: "1.0.0", URL: "/nginx-1.0.0.tgz", Size: 2048}
	src := &fakeSrc{content: map[string][]byte{
		"/nginx-1.0.0.tgz": []byte("chart-bytes"),
	}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)

	if err := job.migrateHelmHTTP(context.Background()); err != nil {
		t.Fatalf("migrateHelmHTTP returned err: %v", err)
	}

	if len(stats.FileStats) != 1 {
		t.Fatalf("expected exactly 1 FileStat (chart only), got %d: %+v", len(stats.FileStats), stats.FileStats)
	}
	if stats.FileStats[0].Status != types.StatusSuccess {
		t.Errorf("expected chart Success, got %s", stats.FileStats[0].Status)
	}
	if len(dest.uploaded) != 1 || dest.uploaded[0] != "nginx-1.0.0.tgz" {
		t.Errorf("dest uploads = %v, want [nginx-1.0.0.tgz]", dest.uploaded)
	}
}

// TestMigrateHelmHTTPChartDownloadFails: source chart download errors → one
// StatusFail FileStat with Error set, prov NOT attempted, nothing uploaded, and
// the method propagates the error.
func TestMigrateHelmHTTPChartDownloadFails(t *testing.T) {
	pkg := types.Package{Name: "nginx", Version: "1.0.0", URL: "/nginx-1.0.0.tgz", Size: 2048}
	// content empty → chart download fails; prov also absent (must not be reached).
	src := &fakeSrc{content: map[string][]byte{}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)

	err := job.migrateHelmHTTP(context.Background())
	if err == nil {
		t.Fatalf("expected error on chart download failure, got nil")
	}

	if len(stats.FileStats) != 1 {
		t.Fatalf("expected exactly 1 FileStat (chart fail), got %d: %+v", len(stats.FileStats), stats.FileStats)
	}
	s := stats.FileStats[0]
	if s.Status != types.StatusFail {
		t.Errorf("expected StatusFail, got %s", s.Status)
	}
	if s.Error == "" {
		t.Errorf("expected Error to be set on download failure")
	}
	if len(dest.uploaded) != 0 {
		t.Errorf("nothing should be uploaded on chart download failure, got %v", dest.uploaded)
	}
}

// TestMigrateHelmHTTPProvUploadFails: chart uploads cleanly but the prov upload
// fails → chart Success + prov StatusFail (the chart is NOT rolled back; the
// prov failure is recorded independently).
func TestMigrateHelmHTTPProvUploadFails(t *testing.T) {
	pkg := types.Package{Name: "nginx", Version: "1.0.0", URL: "/nginx-1.0.0.tgz", Size: 2048}
	src := &fakeSrc{content: map[string][]byte{
		"/nginx-1.0.0.tgz":      []byte("chart-bytes"),
		"/nginx-1.0.0.tgz.prov": []byte("prov-bytes"),
	}}
	dest := &fakeDest{failSuffix: ".prov"}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)

	if err := job.migrateHelmHTTP(context.Background()); err != nil {
		t.Fatalf("migrateHelmHTTP returned err (chart succeeded, only prov failed): %v", err)
	}

	if len(stats.FileStats) != 2 {
		t.Fatalf("expected 2 FileStats (chart Success + prov Fail), got %d: %+v", len(stats.FileStats), stats.FileStats)
	}
	chartStat, provStat := stats.FileStats[0], stats.FileStats[1]
	if chartStat.Status != types.StatusSuccess {
		t.Errorf("chart stat = %s, want Success", chartStat.Status)
	}
	if provStat.Status != types.StatusFail {
		t.Errorf("prov stat = %s, want Fail", provStat.Status)
	}
	if provStat.Error == "" {
		t.Errorf("expected prov Error to be set")
	}
	if !strings.HasSuffix(provStat.Uri, ".prov") {
		t.Errorf("prov stat Uri = %q, want a .prov URL", provStat.Uri)
	}
	// Only the chart should have made it into the dest record.
	if len(dest.uploaded) != 1 || dest.uploaded[0] != "nginx-1.0.0.tgz" {
		t.Errorf("dest uploads = %v, want [nginx-1.0.0.tgz]", dest.uploaded)
	}
}

// TestMigrateHelmHTTPChartUploadFails: source download is fine but the dest
// rejects the chart upload → one StatusFail FileStat, prov NOT attempted.
func TestMigrateHelmHTTPChartUploadFails(t *testing.T) {
	pkg := types.Package{Name: "nginx", Version: "1.0.0", URL: "/nginx-1.0.0.tgz", Size: 2048}
	src := &fakeSrc{content: map[string][]byte{
		"/nginx-1.0.0.tgz":      []byte("chart-bytes"),
		"/nginx-1.0.0.tgz.prov": []byte("prov-bytes"),
	}}
	// Fail the chart (.tgz) upload. failSuffix ".tgz" also matches ".tgz.prov",
	// but the prov upload must never be reached after a chart failure.
	dest := &fakeDest{failSuffix: ".tgz"}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)

	err := job.migrateHelmHTTP(context.Background())
	if err == nil {
		t.Fatalf("expected error on chart upload failure, got nil")
	}

	if len(stats.FileStats) != 1 {
		t.Fatalf("expected exactly 1 FileStat (chart upload fail), got %d: %+v", len(stats.FileStats), stats.FileStats)
	}
	if stats.FileStats[0].Status != types.StatusFail {
		t.Errorf("expected StatusFail, got %s", stats.FileStats[0].Status)
	}
	if len(dest.uploaded) != 0 {
		t.Errorf("no successful uploads expected, got %v", dest.uploaded)
	}
}

// TestMigrateHelmHTTPNestedName: a nested package name (JFrog getNestedName
// output) is preserved into the canonical upload file name verbatim.
func TestMigrateHelmHTTPNestedName(t *testing.T) {
	pkg := types.Package{Name: "ChartA/ChartB/abc", Version: "1.0.1", URL: "/ChartA/ChartB/abc-1.0.1.tgz", Size: 2048}
	src := &fakeSrc{content: map[string][]byte{
		"/ChartA/ChartB/abc-1.0.1.tgz": []byte("chart-bytes"),
	}}
	dest := &fakeDest{}
	stats := &types.TransferStats{}
	job := newHelmHTTPJob(src, dest, pkg, stats)

	if err := job.migrateHelmHTTP(context.Background()); err != nil {
		t.Fatalf("migrateHelmHTTP returned err: %v", err)
	}
	if len(dest.uploaded) != 1 || dest.uploaded[0] != "ChartA/ChartB/abc-1.0.1.tgz" {
		t.Errorf("dest uploads = %v, want [ChartA/ChartB/abc-1.0.1.tgz]", dest.uploaded)
	}
}
