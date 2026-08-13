package upload

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/harness/harness-cli/config"
)

// mockJob is a controllable FileUploadJob for testing Execute.
type mockJob struct {
	BaseFileUploadJob
	uploadFn func(ctx context.Context) error
}

func (m *mockJob) Upload(ctx context.Context) error {
	return m.uploadFn(ctx)
}

func newSuccessJob(id string) *mockJob {
	return &mockJob{
		BaseFileUploadJob: BaseFileUploadJob{ID: id, FilePath: "/fake/" + id, FileSize: 100},
		uploadFn:          func(ctx context.Context) error { return nil },
	}
}

func newFailJob(id string, err error) *mockJob {
	return &mockJob{
		BaseFileUploadJob: BaseFileUploadJob{ID: id, FilePath: "/fake/" + id, FileSize: 100},
		uploadFn:          func(ctx context.Context) error { return err },
	}
}

// mockReporter captures progress calls made by Execute.
// Reporter methods are only called from the main goroutine in Execute,
// so no mutex is required.
type mockReporter struct {
	steps     []string
	errs      []string
	successes []string
}

func (r *mockReporter) Start(msg string)   {}
func (r *mockReporter) Step(msg string)    { r.steps = append(r.steps, msg) }
func (r *mockReporter) Error(msg string)   { r.errs = append(r.errs, msg) }
func (r *mockReporter) Success(msg string) { r.successes = append(r.successes, msg) }
func (r *mockReporter) End()               {}

// newTestEngine creates a FileUploadEngine with a mockReporter and suppresses
// the pterm progress bar so tests run without terminal side-effects.
func newTestEngine(t *testing.T, workers int) (*FileUploadEngine, *mockReporter) {
	t.Helper()
	orig := config.Global
	config.Global.NoProgress = true
	t.Cleanup(func() { config.Global = orig })
	t.Setenv("CI", "true")

	rep := &mockReporter{}
	return NewFileUploadEngine(workers, rep), rep
}

func TestExecute_NilJobs_ReturnsNil(t *testing.T) {
	eng, _ := newTestEngine(t, 2)
	result := eng.Execute(context.Background(), nil)
	if result != nil {
		t.Errorf("expected nil for nil jobs, got %v", result)
	}
}

func TestExecute_EmptySlice_ReturnsNil(t *testing.T) {
	eng, _ := newTestEngine(t, 2)
	result := eng.Execute(context.Background(), []FileUploadJob{})
	if result != nil {
		t.Errorf("expected nil for empty job slice, got %v", result)
	}
}

func TestExecute_AllSucceed(t *testing.T) {
	eng, rep := newTestEngine(t, 3)
	jobs := []FileUploadJob{
		newSuccessJob("a"),
		newSuccessJob("b"),
		newSuccessJob("c"),
	}
	results := eng.Execute(context.Background(), jobs)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("job %s: expected Success=true, got error: %v", r.JobID, r.Error)
		}
	}
	if len(rep.successes) != 1 {
		t.Errorf("expected 1 progress.Success call, got %d", len(rep.successes))
	}
	if len(rep.errs) != 0 {
		t.Errorf("expected no progress.Error calls, got %v", rep.errs)
	}
}

func TestExecute_AllFail(t *testing.T) {
	eng, rep := newTestEngine(t, 2)
	uploadErr := errors.New("upload failed")
	jobs := []FileUploadJob{
		newFailJob("x", uploadErr),
		newFailJob("y", uploadErr),
	}
	results := eng.Execute(context.Background(), jobs)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Success {
			t.Errorf("job %s: expected Success=false", r.JobID)
		}
		if r.Error == nil {
			t.Errorf("job %s: expected non-nil error", r.JobID)
		}
	}
	if len(rep.errs) != 1 {
		t.Errorf("expected 1 progress.Error call, got %d", len(rep.errs))
	}
	if len(rep.successes) != 0 {
		t.Errorf("expected no progress.Success calls, got %v", rep.successes)
	}
}

func TestExecute_PartialFailure(t *testing.T) {
	eng, rep := newTestEngine(t, 3)
	uploadErr := errors.New("partial failure")
	jobs := []FileUploadJob{
		newSuccessJob("ok1"),
		newFailJob("bad1", uploadErr),
		newSuccessJob("ok2"),
	}
	results := eng.Execute(context.Background(), jobs)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	if successCount != 2 {
		t.Errorf("expected 2 successes, got %d", successCount)
	}
	if len(rep.errs) != 1 {
		t.Errorf("expected 1 progress.Error call, got %d", len(rep.errs))
	}
	if !strings.Contains(rep.errs[0], "2/3 succeeded") {
		t.Errorf("expected error message to contain '2/3 succeeded', got %q", rep.errs[0])
	}
	if len(rep.successes) != 0 {
		t.Errorf("expected no progress.Success calls on partial failure, got %v", rep.successes)
	}
}

func TestExecute_WorkerCountCappedToJobCount(t *testing.T) {
	// maxWorkers (10) > len(jobs) (1): numWorkers should be capped to 1
	eng, _ := newTestEngine(t, 10)
	results := eng.Execute(context.Background(), []FileUploadJob{newSuccessJob("solo")})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}
}

func TestExecute_ResultMetadataMatchesJob(t *testing.T) {
	eng, _ := newTestEngine(t, 1)
	job := &mockJob{
		BaseFileUploadJob: BaseFileUploadJob{ID: "job-1", FilePath: "/path/file.bin", FileSize: 512},
		uploadFn:          func(ctx context.Context) error { return nil },
	}
	results := eng.Execute(context.Background(), []FileUploadJob{job})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.JobID != "job-1" {
		t.Errorf("JobID = %q, want 'job-1'", r.JobID)
	}
	if r.FilePath != "/path/file.bin" {
		t.Errorf("FilePath = %q, want '/path/file.bin'", r.FilePath)
	}
	if r.FileSize != 512 {
		t.Errorf("FileSize = %d, want 512", r.FileSize)
	}
}

func TestExecute_StepCalledOnce(t *testing.T) {
	eng, rep := newTestEngine(t, 2)
	jobs := []FileUploadJob{newSuccessJob("a"), newSuccessJob("b")}
	eng.Execute(context.Background(), jobs)

	if len(rep.steps) != 1 {
		t.Errorf("expected 1 progress.Step call, got %d", len(rep.steps))
	}
}

func TestExecute_CancelledContext(t *testing.T) {
	eng, _ := newTestEngine(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// uploadFn propagates ctx cancellation so the test is deterministic
	// regardless of which select branch fires in the worker.
	job := &mockJob{
		BaseFileUploadJob: BaseFileUploadJob{ID: "c", FilePath: "/fake/c", FileSize: 0},
		uploadFn:          func(ctx context.Context) error { return ctx.Err() },
	}
	results := eng.Execute(ctx, []FileUploadJob{job})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected Success=false for cancelled context job")
	}
	if results[0].Error == nil {
		t.Error("expected non-nil error for cancelled context job")
	}
}
