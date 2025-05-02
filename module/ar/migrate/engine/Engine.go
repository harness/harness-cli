package engine

import (
	"context"
	"errors"
	"fmt"
	"harness/module/ar/migrate/migratable"
	"runtime"
	"sync"
)

type Engine struct {
	concurrency int
	jobs        []migratable.Job
}

func NewEngine(concurrency int, jobs []migratable.Job) *Engine {
	return &Engine{
		concurrency: concurrency,
		jobs:        jobs,
	}
}

func (e *Engine) Execute(ctx context.Context) error {
	if len(e.jobs) == 0 {
		return nil
	}
	if e.concurrency <= 0 {
		e.concurrency = runtime.NumCPU()
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
			step := func(name string, fn func(context.Context) error) bool {
				if err := fn(ctx); err != nil {
					errCh <- fmt.Errorf("job %d|%s: %s-step: %w", i, info, name, err)
					return false
				}
				return true
			}
			if !step("pre", jb.Pre) || !step("migrate", jb.Migrate) || !step("post", jb.Post) {
				return
			}
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
