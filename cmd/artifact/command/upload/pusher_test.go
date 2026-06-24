package upload

import (
	"context"
	"testing"

	commonupload "github.com/harness/harness-cli/util/common/upload"
)

// ── UploadStats ───────────────────────────────────────────────────────────────

func TestUploadStats_ZeroValue(t *testing.T) {
	var s UploadStats
	if s.FileCount != 0 {
		t.Errorf("FileCount zero value: got %d, want 0", s.FileCount)
	}
	if s.TotalBytes != 0 {
		t.Errorf("TotalBytes zero value: got %d, want 0", s.TotalBytes)
	}
}

func TestUploadStats_Accumulate(t *testing.T) {
	s := UploadStats{FileCount: 3, TotalBytes: 1024}
	s.FileCount++
	s.TotalBytes += 512
	if s.FileCount != 4 {
		t.Errorf("FileCount: got %d, want 4", s.FileCount)
	}
	if s.TotalBytes != 1536 {
		t.Errorf("TotalBytes: got %d, want 1536", s.TotalBytes)
	}
}

// ── Pusher interface compliance ───────────────────────────────────────────────

// Compile-time check: GenericUploader must satisfy Pusher.
var _ Pusher = (*GenericUploader)(nil)

func TestPusher_GenericUploaderImplementsInterface(t *testing.T) {
	var p Pusher = &GenericUploader{}
	if p == nil {
		t.Fatal("GenericUploader should satisfy Pusher")
	}
}

// mockPusher is used to verify the Pusher interface contract can be implemented
// by any type, not just GenericUploader.
type mockPusher struct {
	getRegistryCallCount int
	getFilesCallCount    int
	pushFilesCallCount   int
	registryResult       string
	fileResult           []commonupload.FileUploadJob
	statsResult          UploadStats
	errResult            error
}

func (m *mockPusher) GetRegistryAndPath(_ string) (string, error) {
	m.getRegistryCallCount++
	return m.registryResult, m.errResult
}

func (m *mockPusher) GetFiles() ([]commonupload.FileUploadJob, UploadStats, error) {
	m.getFilesCallCount++
	return m.fileResult, m.statsResult, m.errResult
}

func (m *mockPusher) PushFiles(_ context.Context, _ []commonupload.FileUploadJob) error {
	m.pushFilesCallCount++
	return m.errResult
}

func TestPusher_MockImplementation(t *testing.T) {
	mock := &mockPusher{
		registryResult: "test-registry",
		statsResult:    UploadStats{FileCount: 2, TotalBytes: 100},
	}
	var p Pusher = mock

	reg, err := p.GetRegistryAndPath("test-registry/path")
	if err != nil {
		t.Fatalf("GetRegistryAndPath: %v", err)
	}
	if reg != "test-registry" {
		t.Errorf("registry: got %q, want test-registry", reg)
	}

	jobs, stats, err := p.GetFiles()
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if stats.FileCount != 2 {
		t.Errorf("FileCount: got %d, want 2", stats.FileCount)
	}
	if len(jobs) != 0 {
		t.Errorf("jobs: got %d, want 0", len(jobs))
	}

	if err := p.PushFiles(context.Background(), nil); err != nil {
		t.Fatalf("PushFiles: %v", err)
	}

	if mock.getRegistryCallCount != 1 || mock.getFilesCallCount != 1 || mock.pushFilesCallCount != 1 {
		t.Errorf("call counts: GetRegistryAndPath=%d GetFiles=%d PushFiles=%d, want all 1",
			mock.getRegistryCallCount, mock.getFilesCallCount, mock.pushFilesCallCount)
	}
}
