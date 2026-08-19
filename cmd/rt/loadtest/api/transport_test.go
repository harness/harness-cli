package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/harness/harness-cli/config"
)

// withToken pins the global auth token for one test. The client carries no
// credential of its own; util/common/auth reads this on every request.
func withToken(t *testing.T, token string) {
	t.Helper()

	previous := config.Global.AuthToken
	t.Cleanup(func() { config.Global.AuthToken = previous })
	config.Global.AuthToken = token
}

// captureHeaders runs one request against a stub server and returns what the
// transport actually sent.
func captureHeaders(t *testing.T, token string) http.Header {
	t.Helper()

	withToken(t, token)

	var sent http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = r.Header.Clone()
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL, Scope: Scope{AccountID: "acct"}})
	if err := client.do(context.Background(), request{Method: http.MethodGet, Path: "/load-tests"}, nil); err != nil {
		t.Fatalf("do: %v", err)
	}

	return sent
}

// A PAT is presented as x-api-key.
func TestPATIsSentAsAnAPIKey(t *testing.T) {
	sent := captureHeaders(t, "pat.acct.0123456789abcdef.value")

	if got := sent.Get("x-api-key"); got != "pat.acct.0123456789abcdef.value" {
		t.Errorf("x-api-key is %q, want the token", got)
	}
	if got := sent.Get("Authorization"); got != "" {
		t.Errorf("a PAT was also sent as Authorization: %q", got)
	}
}

// A CIManager JWT goes to Authorization, as every other client in the CLI
// routes it. Setting x-api-key unconditionally broke only the loadtest commands.
func TestJWTIsSentAsAuthorization(t *testing.T) {
	const jwt = "CIManager eyJhbGciOiJIUzI1NiJ9.payload.signature"
	sent := captureHeaders(t, jwt)

	if got := sent.Get("Authorization"); got != jwt {
		t.Errorf("Authorization is %q, want the JWT", got)
	}
	// Sending both is worse than sending the wrong one: a gateway that
	// length-checks x-api-key rejects the request outright.
	if got := sent.Get("x-api-key"); got != "" {
		t.Errorf("a JWT was also sent as x-api-key: %q", got)
	}
}

// Every other client identifies itself, and the load test one skipped it, so
// its traffic was indistinguishable from an arbitrary script server-side.
func TestRequestsCarryTheCLIUserAgent(t *testing.T) {
	sent := captureHeaders(t, "pat.acct.0123456789abcdef.value")

	if got := sent.Get("User-Agent"); got != config.UserAgent() {
		t.Errorf("User-Agent is %q, want %q", got, config.UserAgent())
	}
}

// attemptsFor counts how many times a method reaches the server when every
// reply is a 500.
func attemptsFor(t *testing.T, method, path string) int {
	t.Helper()

	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	_ = client.do(context.Background(), request{Method: method, Path: path}, nil)

	mu.Lock()
	defer mu.Unlock()
	return attempts
}

// A repeated read is free, so a transient 5xx should not surface.
func TestReadsAreRetried(t *testing.T) {
	if got := attemptsFor(t, http.MethodGet, "/load-tests"); got < 2 {
		t.Errorf("a failing GET was tried %d time(s); reads should be retried", got)
	}
}

// A retried write is a duplicate, not a no-op: repeating POST /runs starts a
// second run against the real target system.
func TestWritesAreNotRetried(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if got := attemptsFor(t, method, "/load-tests/checkout-load/runs"); got != 1 {
			t.Errorf("a failing %s was sent %d times; a write must go exactly once", method, got)
		}
	}
}
