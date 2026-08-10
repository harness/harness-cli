package download

import (
	"context"
	"fmt"
	"sync"
	"time"

	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/pterm/pterm"
)

const (
	DefaultDownloadWorker = 5
)

// manages concurrent file downloads
type FileDownloadEngine struct {
	maxWorkers int
	progress   p.Reporter
}

// creating a new download engine, to perform downloads concurrently
func NewFileDownloadEngine(maxWorkers int, progress p.Reporter) *FileDownloadEngine {
	if maxWorkers <= 0 {
		maxWorkers = DefaultDownloadWorker
	}
	return &FileDownloadEngine{
		maxWorkers: maxWorkers,
		progress:   progress,
	}
}

// Execute runs all download jobs concurrently.
func (e *FileDownloadEngine) Execute(ctx context.Context, jobs []FileDownloadJob) []FileDownloadResult {
	if len(jobs) == 0 {
		return nil
	}

	numWorkers := e.maxWorkers
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	e.progress.Step(fmt.Sprintf("Starting download: %d files with %d workers. Please wait ....", len(jobs), numWorkers))

	// Create progress bar
	progressBar, _ := pterm.DefaultProgressbar.WithTotal(len(jobs)).WithTitle("Downloading files").Start()
	defer func() { _, _ = progressBar.Stop() }()

	startTime := time.Now()
	jobChan := make(chan FileDownloadJob, len(jobs))
	resultChan := make(chan FileDownloadResult, len(jobs))

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
	results := make([]FileDownloadResult, 0, len(jobs))
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
				progressBar.UpdateTitle(fmt.Sprintf("Downloading files (%d/%d completed)", successCount, len(jobs)))
			} else {
				progressBar.UpdateTitle(fmt.Sprintf("Downloading files (%d/%d completed, %d failed)", successCount, len(jobs), len(results)-successCount))
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
		e.progress.Success(fmt.Sprintf("Successfully downloaded %d files in %v (%.2f files/sec)",
			len(jobs), duration, float64(len(jobs))/duration.Seconds()))
	} else {
		failCount := len(jobs) - successCount
		e.progress.Error(fmt.Sprintf("Download completed with errors: %d/%d succeeded, %d failed in %v",
			successCount, len(jobs), failCount, duration))
	}

	return results
}

// worker processes download jobs from the job channel
func (e *FileDownloadEngine) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan FileDownloadJob, results chan<- FileDownloadResult) {
	defer wg.Done()

	for job := range jobs {
		select {
		case <-ctx.Done():
			// Drain the queue and emit a cancellation result for every remaining
			// job so callers see a complete accounting instead of silent drops.
			results <- FileDownloadResult{
				JobID:    job.GetID(),
				FilePath: job.GetFilePath(),
				FileSize: job.GetFileSize(),
				Error:    ctx.Err(),
				Success:  false,
			}
			continue
		default:
			err := job.Download(ctx)
			results <- FileDownloadResult{
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
func HasDownloadErrors(results []FileDownloadResult) bool {
	for _, result := range results {
		if result.Error != nil {
			return true
		}
	}
	return false
}

// returns a map of failed downloads
func GetDownloadErrors(results []FileDownloadResult) map[string]error {
	errs := make(map[string]error)
	for _, result := range results {
		if result.Error != nil {
			errs[result.JobID] = result.Error
		}
	}
	return errs
}

// returns count of successful downloads
func GetSuccessfulDownloads(results []FileDownloadResult) int {
	count := 0
	for _, result := range results {
		if result.Success {
			count++
		}
	}
	return count
}
