package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Engine struct {
	concurrency int
	jobs        []Job
}

func NewEngine(concurrency int, jobs []Job) *Engine {
	return &Engine{
		concurrency: concurrency,
		jobs:        jobs,
	}
}

func (e *Engine) Execute(ctx context.Context) error {
	mainLogger := log.With().
		Int("concurrency", e.concurrency).
		Int("total_jobs", len(e.jobs)).
		Logger()

	if len(e.jobs) == 0 {
		mainLogger.Info().Msg("No jobs to execute")
		return nil
	}

	traceID := uuid.New().String()
	ctx = context.WithValue(ctx, "trace_id", traceID)

	mainLogger = mainLogger.With().Str("trace_id", traceID).Logger()
	mainLogger.Info().Msg("Starting engine execution")
	// Create a trace ID for the entire migration operation

	if e.concurrency <= 0 {
		e.concurrency = runtime.NumCPU()
		mainLogger.Debug().Int("adjusted_concurrency", e.concurrency).Msg("Adjusted concurrency")
	}

	sem := make(chan struct{}, e.concurrency)
	errCh := make(chan error, len(e.jobs))
	var wg sync.WaitGroup

	for i, jb := range e.jobs {
		i, jb := i, jb
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			info := jb.Info()
			jobLogger := mainLogger.With().
				Int("job_index", i).
				Str("job_info", info).
				Logger()

			jobLogger.Debug().Msg("Starting job execution")
			jobStartTime := time.Now()

			step := func(name string, fn func(context.Context) error) bool {
				stepLogger := jobLogger.With().Str("step", name).Logger()
				stepLogger.Debug().Msg("Starting step")
				stepStartTime := time.Now()

				if err := fn(ctx); err != nil {
					stepLogger.Error().
						Err(err).
						Dur("duration", time.Since(stepStartTime)).
						Msg("Step failed")

					errCh <- fmt.Errorf("job %d|%s: %s-step: %w", i, info, name, err)
					return false
				}
				stepLogger.Debug().
					Dur("duration", time.Since(stepStartTime)).
					Msg("Step completed successfully")
				return true
			}

			if !step("pre", jb.Pre) || !step("migrate", jb.Migrate) || !step("post", jb.Post) {
				jobLogger.Warn().
					Dur("duration", time.Since(jobStartTime)).
					Msg("Job execution terminated with errors")
				return
			}

			jobLogger.Info().
				Dur("duration", time.Since(jobStartTime)).
				Msg("Job completed successfully")
		}()
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
