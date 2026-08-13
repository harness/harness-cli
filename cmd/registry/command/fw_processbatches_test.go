package command

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	ar_v3 "github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// stubEvaluator implements Evaluator with controllable per-batch behavior.
// Used to exercise the parallel batch runner without any HTTP layer.
type stubEvaluator struct {
	mode     string
	max      int
	call     int32                                          // parallel-safe call counter
	handler  func(idx int32, batch []Dependency) (batchResult, error)
}

func (s *stubEvaluator) MaxBatchSize() int { return s.max }
func (s *stubEvaluator) Mode() string      { return s.mode }
func (s *stubEvaluator) Evaluate(ctx context.Context, batch []Dependency, info batchInfo) (batchResult, error) {
	idx := atomic.AddInt32(&s.call, 1) - 1
	return s.handler(idx, batch)
}

func makeDeps(n int) []Dependency {
	out := make([]Dependency, n)
	for i := 0; i < n; i++ {
		out[i] = Dependency{Name: "p", Version: "1"}
	}
	return out
}

func TestProcessBatches_AllSuccess(t *testing.T) {
	stub := &stubEvaluator{
		mode: "sync",
		max:  20,
		handler: func(idx int32, batch []Dependency) (batchResult, error) {
			res := make([]ScanResult, len(batch))
			for i := range batch {
				res[i] = ScanResult{PackageName: "p", Version: "1", ScanStatus: "ALLOWED"}
			}
			return batchResult{Results: res}, nil
		},
	}
	ctx := &auditContext{
		p:            progress.NewConsoleReporter(),
		evaluator:    stub,
		batchSize:    5,
		registryUUID: uuid.New(),
	}
	results, err := processBatches(ctx, makeDeps(13), "r")
	assert.NoError(t, err)
	assert.Len(t, results, 13)
}

func TestProcessBatches_MixedFailure(t *testing.T) {
	// Batches 0 and 2 succeed; batch 1 fails and produces UNKNOWNs.
	stub := &stubEvaluator{
		mode: "sync",
		max:  20,
		handler: func(idx int32, batch []Dependency) (batchResult, error) {
			if idx == 1 {
				return batchResult{}, errors.New("simulated batch failure")
			}
			res := make([]ScanResult, len(batch))
			for i := range batch {
				res[i] = ScanResult{PackageName: "p", Version: "1", ScanStatus: "ALLOWED"}
			}
			return batchResult{Results: res}, nil
		},
	}
	ctx := &auditContext{
		p:         progress.NewConsoleReporter(),
		evaluator: stub,
		batchSize: 5,
	}
	results, err := processBatches(ctx, makeDeps(15), "r")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated batch failure")
	assert.Len(t, results, 15)

	unknowns, allowed := 0, 0
	for _, r := range results {
		switch r.ScanStatus {
		case "UNKNOWN":
			unknowns++
		case "ALLOWED":
			allowed++
		}
	}
	assert.Equal(t, 5, unknowns) // one full batch marked UNKNOWN
	assert.Equal(t, 10, allowed) // two batches succeeded
}

func TestProcessBatches_AsyncForcesSerial(t *testing.T) {
	// Async mode must run one batch at a time regardless of the sync worker
	// count. We assert only 1 concurrent invocation ever by tracking an atomic counter.
	var inflight, maxInflight int32
	stub := &stubEvaluator{
		mode: "async",
		max:  50,
		handler: func(idx int32, batch []Dependency) (batchResult, error) {
			n := atomic.AddInt32(&inflight, 1)
			// track max
			for {
				m := atomic.LoadInt32(&maxInflight)
				if n <= m || atomic.CompareAndSwapInt32(&maxInflight, m, n) {
					break
				}
			}
			atomic.AddInt32(&inflight, -1)
			return batchResult{Results: []ScanResult{{ScanStatus: "ALLOWED"}}}, nil
		},
	}
	ctx := &auditContext{
		p:         progress.NewConsoleReporter(),
		evaluator: stub,
		batchSize: 1,
	}
	_, err := processBatches(ctx, makeDeps(5), "r")
	assert.NoError(t, err)
	assert.Equal(t, int32(1), maxInflight, "async mode must be strictly serial")
}

func TestProcessBatches_BatchSizeCapAtMax(t *testing.T) {
	// Requesting batch-size=100 for a max-50 evaluator should clamp to 50.
	var seenBatchSizes []int
	stub := &stubEvaluator{
		mode: "sync",
		max:  50,
		handler: func(idx int32, batch []Dependency) (batchResult, error) {
			seenBatchSizes = append(seenBatchSizes, len(batch))
			return batchResult{Results: make([]ScanResult, len(batch))}, nil
		},
	}
	ctx := &auditContext{
		p:         progress.NewConsoleReporter(),
		evaluator: stub,
		batchSize: 100, // over the max
	}
	_, err := processBatches(ctx, makeDeps(120), "r")
	assert.NoError(t, err)
	// 120 / 50 = 2 batches of 50 + 1 of 20
	assert.Equal(t, []int{50, 50, 20}, seenBatchSizes)
}

// TestV3DetailsToScanDetails checks the adapter used by fw explain when
// feeding the sync response into displayScanDetails.
func TestV3DetailsToScanDetails(t *testing.T) {
	pt := ar_v3.PackageType("NPM")
	name := "npm-registry"
	last := "1706000000000"
	v3 := &ar_v3.ArtifactScanDetailsV3{
		PackageName:     "express",
		Version:         "4.18.2",
		PackageType:     &pt,
		RegistryName:    &name,
		LastEvaluatedAt: &last,
		ScanStatus:      ar_v3.ArtifactScanDetailsV3ScanStatusBLOCKED,
	}
	adapted := v3DetailsToScanDetails(v3)
	assert.Equal(t, "express", adapted.PackageName)
	assert.Equal(t, "4.18.2", adapted.Version)
	assert.Equal(t, ar_v3.PackageType("NPM"), adapted.PackageType)
	assert.Equal(t, "npm-registry", adapted.RegistryName)
	assert.NotNil(t, adapted.LastEvaluatedAt)
	assert.Equal(t, "1706000000000", *adapted.LastEvaluatedAt)
}
