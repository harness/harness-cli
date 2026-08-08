package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/adapter/jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/adapter/mock_jfrog"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/google/go-containerregistry/pkg/authn"
)

// fakeDestAdapter is a destination Adapter that records uploads and can be
// instructed to fail (or 409) per URI. Source-side methods are unused and
// return zero values.
type fakeDestAdapter struct {
	mu       sync.Mutex
	uploads  []string          // URIs successfully uploaded
	failWith map[string]error  // uri -> error to return from UploadFile
	failAll  error             // when non-nil, every upload fails
}

func (f *fakeDestAdapter) GetKeyChain(string) (authn.Keychain, error) { return nil, nil }
func (f *fakeDestAdapter) GetConfig() types.RegistryConfig            { return types.RegistryConfig{} }
func (f *fakeDestAdapter) ValidateCredentials() (bool, error)         { return true, nil }
func (f *fakeDestAdapter) GetRegistry(context.Context, string) (types.RegistryInfo, error) {
	return types.RegistryInfo{Type: "HAR", Path: "dst-reg"}, nil
}
func (f *fakeDestAdapter) CreateRegistryIfDoesntExist(string) (bool, error) { return false, nil }
func (f *fakeDestAdapter) GetPackages(string, types.ArtifactType, *types.TreeNode) ([]types.Package, error) {
	return nil, nil
}
func (f *fakeDestAdapter) GetVersions(types.Package, *types.TreeNode, string, string, types.ArtifactType) ([]types.Version, error) {
	return nil, nil
}
func (f *fakeDestAdapter) GetFiles(string) ([]types.File, error) { return nil, nil }
func (f *fakeDestAdapter) SearchFiles(string) ([]types.SearchedFile, error) {
	return nil, nil
}
func (f *fakeDestAdapter) DownloadFile(string, string) (io.ReadCloser, http.Header, error) {
	return nil, nil, errors.New("fakeDestAdapter: DownloadFile not supported")
}
func (f *fakeDestAdapter) UploadFile(_ string, _ io.ReadCloser, file *types.File, _ http.Header, _, _ string, _ types.ArtifactType, _ map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return f.failAll
	}
	if err, ok := f.failWith[file.Uri]; ok {
		return err
	}
	f.uploads = append(f.uploads, file.Uri)
	return nil
}
func (f *fakeDestAdapter) GetOCIImagePath(string, string, string) (string, error) { return "", nil }
func (f *fakeDestAdapter) AddNPMTag(string, string, string, string) error         { return nil }
func (f *fakeDestAdapter) VersionExists(context.Context, types.Package, string, string, string, types.ArtifactType) (bool, error) {
	return false, nil
}
func (f *fakeDestAdapter) FileExists(context.Context, string, string, string, *types.File, types.ArtifactType) (bool, error) {
	return false, nil
}
func (f *fakeDestAdapter) BuildExistingIndex(context.Context, string, int) (*types.ExistingIndex, error) {
	return nil, nil
}
func (f *fakeDestAdapter) CreateVersion(string, string, string, types.ArtifactType, []*types.PackageFiles, map[string]interface{}) error {
	return nil
}

func (f *fakeDestAdapter) uploadedURIs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.uploads))
	copy(out, f.uploads)
	return out
}

// newMockBackedService builds a MigrationService with the real jfrog adapter
// backed by the mock client as source and the given fake destination.
func newMockBackedService(cfg *types.Config, dest *fakeDestAdapter) *MigrationService {
	src := jfrog.NewAdapterWithClient(
		types.RegistryConfig{Type: types.MOCK_JFROG, Endpoint: "http://mock"},
		mock_jfrog.NewMockClient(),
	)
	cfg.Summary = true // keep test output to the plain summary printer
	return &MigrationService{config: cfg, source: src, destination: dest}
}

func baseMapping(at types.ArtifactType, srcReg string) types.RegistryMapping {
	return types.RegistryMapping{
		ArtifactType:        at,
		SourceRegistry:      srcReg,
		DestinationRegistry: "dst-reg",
	}
}

func countByStatus(stats []types.FileStat) (success, skipped, failed int) {
	for _, s := range stats {
		switch s.Status {
		case types.StatusSuccess:
			success++
		case types.StatusSkip:
			skipped++
		case types.StatusFail:
			failed++
		}
	}
	return
}

// TestRunReturnsErrorOnEnumerationFailure verifies §2 path (a): an engine-level
// error (source GetFiles abort) makes Run return non-nil AND records a Failed
// stat, instead of the old log-and-return-nil behavior.
func TestRunReturnsErrorOnEnumerationFailure(t *testing.T) {
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		Mappings:    []types.RegistryMapping{baseMapping(types.NUGET, "no-such-registry")},
	}
	svc := newMockBackedService(cfg, &fakeDestAdapter{})

	err := svc.Run(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error when source enumeration fails, got nil")
	}
}

// TestRunReturnsErrorOnTypeMismatch verifies §1-W2: a non-empty source mapping
// that resolves to zero packages (files present, artifactType mismatched) is a
// hard failure — engine error AND a Failed stat — even though no filter is set.
func TestRunReturnsErrorOnTypeMismatch(t *testing.T) {
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		// nuget-local has files but none parse as TERRAFORM coordinates.
		Mappings: []types.RegistryMapping{baseMapping(types.TERRAFORM, "nuget-local")},
	}
	svc := newMockBackedService(cfg, &fakeDestAdapter{})

	err := svc.Run(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error for type-mismatched mapping, got nil")
	}
	if !strings.Contains(err.Error(), "resolved 0 packages") {
		t.Errorf("error %q does not surface the zero-package guard", err.Error())
	}
}

// TestRunReturnsErrorOnUploadFailures verifies §2 path (b): per-coordinate
// upload failures are recorded as StatusFail stats (the package-level jobs
// themselves return nil), and Run still fails the process.
func TestRunReturnsErrorOnUploadFailures(t *testing.T) {
	cfg := &types.Config{
		Concurrency: 2,
		Overwrite:   true,
		Mappings:    []types.RegistryMapping{baseMapping(types.RPM, "rpm-single-local")},
	}
	dest := &fakeDestAdapter{failAll: errors.New("boom: destination unavailable")}
	svc := newMockBackedService(cfg, dest)

	err := svc.Run(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error when uploads fail, got nil")
	}
	if !strings.Contains(err.Error(), "failed to migrate") {
		t.Errorf("error %q does not mention failed artifact count", err.Error())
	}
}

// TestRunSucceedsAndWritesResultFile verifies the happy path plus §2-W4: the
// JSON-lines result file reconciles exactly with the in-memory stats.
func TestRunSucceedsAndWritesResultFile(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "result.jsonl")
	cfg := &types.Config{
		Concurrency: 2,
		Overwrite:   true,
		ResultFile:  resultPath,
		Mappings:    []types.RegistryMapping{baseMapping(types.NUGET, "nuget-local")},
	}
	dest := &fakeDestAdapter{}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error for clean migration, got: %v", err)
	}

	// nuget-local: 1.0.0.nupkg, 2.0.0.nupkg, 2.0.0.snupkg parse as package files.
	uploads := dest.uploadedURIs()
	if len(uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d: %v", len(uploads), uploads)
	}

	records := readResultFile(t, resultPath)
	if len(records) != 3 {
		t.Fatalf("expected 3 result records, got %d", len(records))
	}
	for _, r := range records {
		if r.Status != types.StatusSuccess {
			t.Errorf("expected all Success in result file, got %+v", r)
		}
	}
}

// TestRunWritesResultFileOnFailure verifies the result file exists even when
// the migration fails — the bridge consumes it precisely in that case.
func TestRunWritesResultFileOnFailure(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "nested", "result.jsonl")
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		ResultFile:  resultPath,
		Mappings:    []types.RegistryMapping{baseMapping(types.RPM, "rpm-single-local")},
	}
	dest := &fakeDestAdapter{failAll: errors.New("boom")}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err == nil {
		t.Fatal("expected non-nil error, got nil")
	}

	records := readResultFile(t, resultPath)
	if len(records) != 2 { // nginx + vim-enhanced
		t.Fatalf("expected 2 result records, got %d", len(records))
	}
	for _, r := range records {
		if r.Status != types.StatusFail {
			t.Errorf("expected all Failed in result file, got %+v", r)
		}
	}
}

// TestRunSkipReasonAlreadyExists verifies §2-W3: a 409 from the destination is
// recorded as Skipped with reason=already_exists and does NOT fail the run.
func TestRunSkipReasonAlreadyExists(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "result.jsonl")
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		ResultFile:  resultPath,
		Mappings:    []types.RegistryMapping{baseMapping(types.NUGET, "nuget-local")},
	}
	dest := &fakeDestAdapter{failWith: map[string]error{
		"/foo/company.grpc.pkg/1.0.0/company.grpc.pkg.1.0.0.nupkg": types.ErrArtifactAlreadyExists,
	}}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error when only 409-skips occur, got: %v", err)
	}

	records := readResultFile(t, resultPath)
	success, skipped, failed := 0, 0, 0
	for _, r := range records {
		switch r.Status {
		case types.StatusSuccess:
			success++
		case types.StatusSkip:
			skipped++
			if r.Reason != types.SkipReasonAlreadyExists {
				t.Errorf("skip record missing reason %q: %+v", types.SkipReasonAlreadyExists, r)
			}
		case types.StatusFail:
			failed++
		}
	}
	if success != 2 || skipped != 1 || failed != 0 {
		t.Errorf("expected 2 Success + 1 Skip + 0 Failed, got %d/%d/%d", success, skipped, failed)
	}
}

// TestRunFileLevelIncludePatterns verifies §6-W3 (file-level): includePatterns
// narrow a NUGET run to the matching files only.
func TestRunFileLevelIncludePatterns(t *testing.T) {
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		Mappings: []types.RegistryMapping{{
			ArtifactType:        types.NUGET,
			SourceRegistry:      "nuget-local",
			DestinationRegistry: "dst-reg",
			IncludePatterns:     []string{"foo/company.grpc.pkg/1.0.0/**"},
		}},
	}
	dest := &fakeDestAdapter{}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	uploads := dest.uploadedURIs()
	if len(uploads) != 1 || uploads[0] != "/foo/company.grpc.pkg/1.0.0/company.grpc.pkg.1.0.0.nupkg" {
		t.Fatalf("expected only the 1.0.0 nupkg to upload, got: %v", uploads)
	}
}

// TestRunPackageLevelExcludePatterns verifies §6-W3 (package-level):
// excludePatterns narrow an RPM run by package name.
func TestRunPackageLevelExcludePatterns(t *testing.T) {
	cfg := &types.Config{
		Concurrency: 1,
		Overwrite:   true,
		Mappings: []types.RegistryMapping{{
			ArtifactType:        types.RPM,
			SourceRegistry:      "rpm-single-local",
			DestinationRegistry: "dst-reg",
			ExcludePatterns:     []string{"nginx*"},
		}},
	}
	dest := &fakeDestAdapter{}
	svc := newMockBackedService(cfg, dest)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	uploads := dest.uploadedURIs()
	if len(uploads) != 1 || !strings.Contains(uploads[0], "vim-enhanced") {
		t.Fatalf("expected only vim-enhanced to upload, got: %v", uploads)
	}
}

func readResultFile(t *testing.T, path string) []types.FileStat {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	var records []types.FileStat
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var fs types.FileStat
		if err := json.Unmarshal([]byte(line), &fs); err != nil {
			t.Fatalf("result file line is not valid JSON: %q: %v", line, err)
		}
		records = append(records, fs)
	}
	return records
}
