package types

import (
	"fmt"
	"sync"
	"testing"
)

// TestTransferStatsAddConcurrent verifies that concurrent calls to Add are
// race-free and that every call is durably recorded. Run with `go test -race`
// to prove the mutex actually guards FileStats.
func TestTransferStatsAddConcurrent(t *testing.T) {
	const n = 1000

	stats := &TransferStats{}

	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			stats.Add(FileStat{
				Name:   fmt.Sprintf("file-%d", i),
				Status: StatusSuccess,
			})
		})
	}
	wg.Wait()

	got := stats.Snapshot()
	if len(got) != n {
		t.Fatalf("expected %d file stats, got %d", n, len(got))
	}
}

// TestTransferStatsSnapshotNil verifies Snapshot never returns nil, even for
// a fresh/nil TransferStats, so downstream marshalling never panics.
func TestTransferStatsSnapshotNil(t *testing.T) {
	var stats *TransferStats
	got := stats.Snapshot()
	if got == nil {
		t.Fatal("expected non-nil empty slice from nil TransferStats.Snapshot()")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(got))
	}

	fresh := &TransferStats{}
	got = fresh.Snapshot()
	if got == nil {
		t.Fatal("expected non-nil empty slice from fresh TransferStats.Snapshot()")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(got))
	}
}

// TestTransferStatsSnapshotIndependentCopy verifies mutating the returned
// slice does not affect the internal FileStats.
func TestTransferStatsSnapshotIndependentCopy(t *testing.T) {
	stats := &TransferStats{}
	stats.Add(FileStat{Name: "original", Status: StatusSuccess})

	snap := stats.Snapshot()
	snap[0].Name = "mutated"

	again := stats.Snapshot()
	if again[0].Name != "original" {
		t.Fatalf("expected internal FileStats to remain unaffected by mutation of snapshot, got %q", again[0].Name)
	}
}
