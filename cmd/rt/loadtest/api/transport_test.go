package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// An installation that publishes loadTestManager under a different gateway
// prefix has no other way to reach it, so the route is read from the
// environment rather than compiled in.
func TestTheRoutePrefixCanBeOverriddenFromTheEnvironment(t *testing.T) {
	cases := map[string]string{
		"/custom/loadtest/v2":  "/custom/loadtest/v2",
		"custom/loadtest/v2":   "/custom/loadtest/v2", // written without a leading slash
		"/custom/loadtest/v2/": "/custom/loadtest/v2", // and with a trailing one
		"  ":                   APIPath,               // blank is not an override
	}

	for override, want := range cases {
		t.Setenv(APIPathEnv, override)

		var path string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			_, _ = w.Write([]byte(`{}`))
		}))

		client := NewClient(Config{Server: server.URL})
		_ = client.do(context.Background(), request{Method: http.MethodGet, Path: "/load-tests"}, nil)
		server.Close()

		if got := want + "/load-tests"; path != got {
			t.Errorf("%s=%q routed to %q, want %q", APIPathEnv, override, path, got)
		}
	}
}

// A gateway that answers a JSON route with an HTML error page would otherwise
// leave the caller with a zero-valued struct and no error at all.
func TestABodyThatIsNotJSONIsReportedAgainstTheRouteThatReturnedIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>proxy error</html>"))
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})

	var out LoadTest
	err := client.do(context.Background(), request{Method: http.MethodGet, Path: "/load-tests/checkout-load"}, &out)
	if err == nil {
		t.Fatal("an HTML body decoded into a load test without complaint")
	}
	if !strings.Contains(err.Error(), "/load-tests/checkout-load") {
		t.Errorf("the error does not say which route returned it: %v", err)
	}
}

// A base URL is taken from configuration, so it can be pasted in wrong. The
// complaint has to name the request rather than surface as a bare parse error.
func TestAnUnusableBaseURLIsReportedBeforeAnythingIsSent(t *testing.T) {
	client := NewClient(Config{Server: "://no-scheme"})

	err := client.do(context.Background(), request{Method: http.MethodGet, Path: "/load-tests"}, nil)
	if err == nil {
		t.Fatal("an unparseable base URL was accepted")
	}
	if !strings.Contains(err.Error(), "/load-tests") {
		t.Errorf("the error does not name the request: %v", err)
	}
}

// A request body carries an opaque toolConfig assembled from flags and a config
// file. Something in it that will not encode has to be reported against the
// route rather than panic or go out as a truncated document.
func TestABodyThatWillNotEncodeIsReportedAgainstItsRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a request that could not be encoded was still sent to %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	err := client.do(context.Background(), request{
		Method: http.MethodPost,
		Path:   "/load-tests",
		Body:   map[string]any{"toolConfig": make(chan int)},
	}, nil)

	if err == nil {
		t.Fatal("a body that cannot be encoded was accepted")
	}
	if !strings.Contains(err.Error(), "/load-tests") {
		t.Errorf("the error does not name the route: %v", err)
	}
}

// A method with a space in it cannot go into a request line. It comes from the
// client's own call sites rather than from the user, so this is a guard against
// a typo shipping as a silent no-op.
func TestAnUnusableMethodIsRejected(t *testing.T) {
	client := NewClient(Config{Server: "http://127.0.0.1:1"})

	err := client.do(context.Background(), request{Method: "GET POST", Path: "/load-tests"}, nil)
	if err == nil {
		t.Fatal("a malformed method was accepted")
	}
}

// A connection dropped part-way through the body is not the same as an empty
// answer: decoding what arrived would report whatever fields made it.
func TestATruncatedResponseIsReportedAgainstItsRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Promise more than is written, then hang up, so the read fails
		// rather than returning a short body.
		w.Header().Set("Content-Length", "512")
		_, _ = w.Write([]byte(`{"identity":"checkout-`))

		hijacked, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijacking the connection: %v", err)
			return
		}
		hijacked.Close()
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})

	var out LoadTest
	err := client.do(context.Background(), request{Method: http.MethodGet, Path: "/load-tests/checkout-load"}, &out)
	if err == nil {
		t.Fatalf("a truncated body decoded into %+v without complaint", out)
	}
	if !strings.Contains(err.Error(), "/load-tests/checkout-load") {
		t.Errorf("the error does not name the route: %v", err)
	}
}
