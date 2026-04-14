package upload

import (
	"context"
	"fmt"
	"sync"
	"time"

	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/pterm/pterm"
)

const (
	DefaultUploadWorker = 5
)

// manages concurrent file uploads
type FileUploadEngine struct {
	maxWorkers int
	progress   p.Reporter
}

// createing a new upload engine , to perform upload concurrently
func NewFileUploadEngine(maxWorkers int, progress p.Reporter) *FileUploadEngine {
	if maxWorkers <= 0 {
		maxWorkers = DefaultUploadWorker
	}
	return &FileUploadEngine{
		maxWorkers: maxWorkers,
		progress:   progress,
	}
}

// Execute runs all upload jobs concurrently
func (e *FileUploadEngine) Execute(ctx context.Context, jobs []FileUploadJob) []FileUploadResult {
	if len(jobs) == 0 {
		return nil
	}

	numWorkers := e.maxWorkers
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	e.progress.Step(fmt.Sprintf("Starting upload: %d files with %d workers. Please wait ....", len(jobs), numWorkers))

	// Create progress bar
	progressBar, _ := pterm.DefaultProgressbar.WithTotal(len(jobs)).WithTitle("Uploading files").Start()
	defer progressBar.Stop()

	startTime := time.Now()
	jobChan := make(chan FileUploadJob, len(jobs))
	resultChan := make(chan FileUploadResult, len(jobs))

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		go e.worker(ctx, &wg, jobChan, resultChan)
	}

	// Send jobs to workers
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Collect results in a separate goroutine and update progress bar
	results := make([]FileUploadResult, 0, len(jobs))
	successCount := 0
	var resultMu sync.Mutex
	var collectorWg sync.WaitGroup

	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for result := range resultChan {
			resultMu.Lock()
			results = append(results, result)
			if result.Success {
				successCount++
				progressBar.UpdateTitle(fmt.Sprintf("Uploading files (%d/%d completed)", len(results), len(jobs)))
			} else {
				progressBar.UpdateTitle(fmt.Sprintf("Uploading files (%d/%d completed, %d failed)", successCount, len(jobs), len(results)-successCount))
			}
			progressBar.Increment()
			resultMu.Unlock()
		}
	}()

	// Wait for all workers to finish
	wg.Wait()
	//closing of result channel
	close(resultChan)

	// Wait for result collector to finish processing all results
	collectorWg.Wait()

	duration := time.Since(startTime)

	// Report summary
	if successCount == len(jobs) {
		e.progress.Success(fmt.Sprintf("Successfully uploaded %d files in %v (%.2f files/sec)",
			len(jobs), duration, float64(len(jobs))/duration.Seconds()))
	} else {
		failCount := len(jobs) - successCount
		e.progress.Error(fmt.Sprintf("Upload completed with errors: %d/%d succeeded, %d failed in %v",
			successCount, len(jobs), failCount, duration))
	}

	return results
}

// worker processes upload jobs from the job channel
func (e *FileUploadEngine) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan FileUploadJob, results chan<- FileUploadResult) {
	defer wg.Done()

	for job := range jobs {
		select {
		case <-ctx.Done():
			results <- FileUploadResult{
				JobID:    job.GetID(),
				FilePath: job.GetFilePath(),
				FileSize: job.GetFileSize(),
				Error:    ctx.Err(),
				Success:  false,
			}
			return
		default:
			err := job.Upload(ctx)
			results <- FileUploadResult{
				JobID:    job.GetID(),
				FilePath: job.GetFilePath(),
				FileSize: job.GetFileSize(),
				Error:    err,
				Success:  err == nil,
			}
		}
	}
}

// checks if any results contain errors
func HasUploadErrors(results []FileUploadResult) bool {
	for _, result := range results {
		if result.Error != nil {
			return true
		}
	}
	return false
}

// this will  returns a map of failed uploads
func GetUploadErrors(results []FileUploadResult) map[string]error {
	errors := make(map[string]error)
	for _, result := range results {
		if result.Error != nil {
			errors[result.JobID] = result.Error
		}
	}
	return errors
}

// returns count of successful uploads
func GetSuccessfulUploads(results []FileUploadResult) int {
	count := 0
	for _, result := range results {
		if result.Success {
			count++
		}
	}
	return count
}
