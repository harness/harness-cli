package download

import (
	"context"
	"errors"
	"testing"

	"github.com/harness/harness-cli/util/common/progress"
)

// mockJob is a minimal FileDownloadJob for engine tests.
type mockJob struct {
	id       string
	filePath string
	err      error
}

func (m *mockJob) GetID() string       { return m.id }
func (m *mockJob) GetFilePath() string { return m.filePath }
func (m *mockJob) GetFileSize() int64  { return 0 }
func (m *mockJob) Download(_ context.Context) error {
	return m.err
}

func newEngine() *FileDownloadEngine {
	return NewFileDownloadEngine(2, progress.NewNopReporter())
}

func TestNewFileDownloadEngine_DefaultsWorkers(t *testing.T) {
	e := NewFileDownloadEngine(0, progress.NewNopReporter())
	if e.maxWorkers != DefaultDownloadWorker {
		t.Errorf("expected default workers %d, got %d", DefaultDownloadWorker, e.maxWorkers)
	}
}

// TestExecute_FewerJobsThanWorkers exercises the branch where the engine
// caps numWorkers to len(jobs) when the job count is smaller than maxWorkers.
func TestExecute_FewerJobsThanWorkers(t *testing.T) {
	engine := NewFileDownloadEngine(10, progress.NewNopReporter())
	jobs := []FileDownloadJob{
		&mockJob{id: "only", filePath: "/dest/only.txt"},
	}
	results := engine.Execute(context.Background(), jobs)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}
}

func TestExecute_EmptyJobs(t *testing.T) {
	results := newEngine().Execute(context.Background(), nil)
	if results != nil {
		t.Errorf("expected nil for empty jobs, got %v", results)
	}
}

func TestExecute_AllSucceed(t *testing.T) {
	jobs := []FileDownloadJob{
		&mockJob{id: "a", filePath: "/dest/a.txt"},
		&mockJob{id: "b", filePath: "/dest/b.txt"},
		&mockJob{id: "c", filePath: "/dest/c.txt"},
	}
	results := newEngine().Execute(context.Background(), jobs)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("expected success for job %s, got error: %v", r.JobID, r.Error)
		}
	}
}

func TestExecute_PartialFailure(t *testing.T) {
	downloadErr := errors.New("connection refused")
	jobs := []FileDownloadJob{
		&mockJob{id: "ok", filePath: "/dest/ok.txt"},
		&mockJob{id: "fail", filePath: "/dest/fail.txt", err: downloadErr},
	}
	results := newEngine().Execute(context.Background(), jobs)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if GetSuccessfulDownloads(results) != 1 {
		t.Errorf("expected 1 successful download, got %d", GetSuccessfulDownloads(results))
	}
	if !HasDownloadErrors(results) {
		t.Error("expected HasDownloadErrors to be true")
	}
}

func TestExecute_AllFail(t *testing.T) {
	downloadErr := errors.New("server error")
	jobs := []FileDownloadJob{
		&mockJob{id: "a", err: downloadErr},
		&mockJob{id: "b", err: downloadErr},
	}
	results := newEngine().Execute(context.Background(), jobs)
	if GetSuccessfulDownloads(results) != 0 {
		t.Errorf("expected 0 successful downloads, got %d", GetSuccessfulDownloads(results))
	}
	if !HasDownloadErrors(results) {
		t.Error("expected HasDownloadErrors to be true")
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// With more jobs than workers, we must confirm every queued job still
	// produces a cancellation result — an early return would silently drop
	// the tail of the queue.
	jobs := []FileDownloadJob{
		&mockJob{id: "a"},
		&mockJob{id: "b"},
		&mockJob{id: "c"},
		&mockJob{id: "d"},
		&mockJob{id: "e"},
	}
	results := newEngine().Execute(ctx, jobs)
	if len(results) != len(jobs) {
		t.Fatalf("expected %d results on cancellation, got %d", len(jobs), len(results))
	}
	for _, r := range results {
		if r.Success {
			t.Errorf("expected all results to be failures on cancellation, got success for %s", r.JobID)
		}
		if r.Error == nil {
			t.Errorf("expected error on cancellation for %s, got nil", r.JobID)
		}
	}
}

func TestHasDownloadErrors_False(t *testing.T) {
	results := []FileDownloadResult{
		{JobID: "a", Success: true},
		{JobID: "b", Success: true},
	}
	if HasDownloadErrors(results) {
		t.Error("expected no errors")
	}
}

func TestGetDownloadErrors_ReturnsOnlyFailed(t *testing.T) {
	err := errors.New("timeout")
	results := []FileDownloadResult{
		{JobID: "a", Success: true},
		{JobID: "b", Success: false, Error: err},
	}
	errs := GetDownloadErrors(results)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs["b"] != err {
		t.Errorf("expected error for job b, got %v", errs["b"])
	}
}

func TestGetSuccessfulDownloads_Count(t *testing.T) {
	results := []FileDownloadResult{
		{Success: true},
		{Success: false},
		{Success: true},
	}
	if got := GetSuccessfulDownloads(results); got != 2 {
		t.Errorf("expected 2 successful downloads, got %d", got)
	}
}
