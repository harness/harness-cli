package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Run identities share a unique index with load tests, so reusing the load
// test's own name works once and then collides.
func TestRunIdentityIsDerivedFromTheLoadTestButNotEqualToIt(t *testing.T) {
	const parent = "checkout-load"

	first := NewRunIdentity(parent)
	if first == parent {
		t.Fatalf("run identity equals the load test identity: %q", first)
	}
	if !strings.HasPrefix(first, parent+"-") {
		t.Fatalf("run identity %q does not identify its load test", first)
	}

	suffix := strings.TrimPrefix(first, parent+"-")
	if len(suffix) != runIdentitySuffixLen {
		t.Fatalf("suffix %q is %d characters, want %d", suffix, len(suffix), runIdentitySuffixLen)
	}
	for _, char := range suffix {
		if !strings.ContainsRune(runIdentityAlphabet, char) {
			t.Fatalf("suffix %q contains %q, which is outside the identity alphabet", suffix, char)
		}
	}
}

// Two runs started in a row must not be handed the same name.
func TestRunIdentitiesDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[NewRunIdentity("checkout-load")] = true
	}
	// 36^3 values drawn 200 times will repeat; the point is that it varies.
	if len(seen) < 100 {
		t.Fatalf("200 generated identities produced only %d distinct values", len(seen))
	}
}

// The duplicate-key rejection arrives as a 500 quoting MongoDB; retrying with a
// fresh name is what stops that reaching the caller.
func TestCreateRunRetriesADuplicateIdentity(t *testing.T) {
	var mu sync.Mutex
	var bodies []CreateRunRequest
	taken := "" // filled with the first identity offered, to reject it once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body CreateRunRequest
		_ = json.NewDecoder(r.Body).Decode(&body)

		mu.Lock()
		bodies = append(bodies, body)
		first := taken == ""
		if first {
			taken = body.Identity
		}
		mu.Unlock()

		if first {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"failed to create load test run: write exception: write errors: ` +
				`[E11000 duplicate key error collection: harness-chaos.loadTestRuns index: ` +
				`account_org_project_identity_unique_partial_idx]"}`))
			return
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Run{Identity: body.Identity, Status: "Pending"})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	run, err := client.CreateRun(context.Background(), "checkout-load", CreateRunRequest{Name: "second"})
	if err != nil {
		t.Fatalf("CreateRun after a duplicate identity: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("made %d attempts, want 2", len(bodies))
	}
	if bodies[0].Identity == bodies[1].Identity {
		t.Fatalf("retried with the same identity %q", bodies[0].Identity)
	}
	if run.Identity != bodies[1].Identity {
		t.Fatalf("returned run %q, want the one accepted, %q", run.Identity, bodies[1].Identity)
	}
	// The load test's identity is restored from what was posted to, so a run
	// never reports its parent as an internal id the caller cannot use.
	if run.LoadTestIdentity != "checkout-load" {
		t.Fatalf("run reports parent %q, want %q", run.LoadTestIdentity, "checkout-load")
	}
}

// An identity supplied by the caller is theirs to own: pinning it and having
// it silently replaced would make the flag a lie.
func TestCreateRunKeepsAnExplicitIdentity(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body CreateRunRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = body.Identity
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Run{Identity: body.Identity})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	if _, err := client.CreateRun(context.Background(), "checkout-load",
		CreateRunRequest{Identity: "nightly-baseline"}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if got != "nightly-baseline" {
		t.Fatalf("posted identity %q, want the one supplied", got)
	}
}

// GET /runs/{identity} names the load test by its internal unique id, which
// rerun would then post back and get a 404 for.
func TestGetRunReportsTheParentByIdentity(t *testing.T) {
	const uniqueID = "3f8e0a8b-66d2-4951-af3a-df0d77654c3b"

	var listings int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/runs/checkout-load-qqm"):
			_ = json.NewEncoder(w).Encode(Run{
				Identity:         "checkout-load-qqm",
				LoadTestIdentity: uniqueID,
			})
		case strings.HasSuffix(r.URL.Path, "/load-tests"):
			listings++
			_ = json.NewEncoder(w).Encode(LoadTestList{
				Items: []*LoadTest{
					{UniqueID: "11111111-1111-1111-1111-111111111111", Identity: "other"},
					{UniqueID: uniqueID, Identity: "checkout-load"},
				},
			})
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	run, err := client.GetRun(context.Background(), "checkout-load-qqm")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.LoadTestIdentity != "checkout-load" {
		t.Fatalf("run reports parent %q, want %q", run.LoadTestIdentity, "checkout-load")
	}

	// Watching a run reads it every few seconds; the mapping cannot change
	// under a live client, so it is looked up once.
	if _, err := client.GetRun(context.Background(), "checkout-load-qqm"); err != nil {
		t.Fatalf("second GetRun: %v", err)
	}
	if listings != 1 {
		t.Fatalf("listed load tests %d times, want 1", listings)
	}
}

// A load test that has been deleted cannot be named, and failing the read over
// it would lose the run's own details, which are all still valid.
func TestGetRunSurvivesAnUnresolvableParent(t *testing.T) {
	const uniqueID = "3f8e0a8b-66d2-4951-af3a-df0d77654c3b"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/load-tests") {
			_ = json.NewEncoder(w).Encode(LoadTestList{})
			return
		}
		_ = json.NewEncoder(w).Encode(Run{Identity: "orphan-run", LoadTestIdentity: uniqueID, Status: "Finished"})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	run, err := client.GetRun(context.Background(), "orphan-run")
	if err != nil {
		t.Fatalf("GetRun with a deleted load test: %v", err)
	}
	if run.Status != "Finished" {
		t.Fatalf("lost the run's own state: %+v", run)
	}
	if run.LoadTestIdentity != uniqueID {
		t.Fatalf("parent became %q, want it left as the unresolvable id", run.LoadTestIdentity)
	}
}

// Stop and retune answer with an acknowledgement, not the run. Decoding one
// into a Run prints a correct header over a row of empty columns.
func TestStopRunReturnsTheRunNotTheAcknowledgement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "stopped"})
			return
		}
		_ = json.NewEncoder(w).Encode(Run{Identity: "checkout-load-71f", Status: "Stopping", TargetUsers: 50})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	run, err := client.StopRun(context.Background(), "checkout-load-71f")
	if err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	if run.Identity == "" || run.Status != "Stopping" {
		t.Fatalf("StopRun returned an empty run: %+v", run)
	}
}
