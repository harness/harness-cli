package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
	"github.com/harness/harness-cli/config"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// These tests drive the real cobra commands against a stub loadTestManager
// rather than calling helpers directly. A command's job is mostly to turn flags
// into a request and a response into output, and only executing it end to end
// exercises that. It also means a flag renamed without its reader being updated
// fails here, which a test calling the helper directly would not catch.

// call identifies a stubbed route. The path is what follows the gateway prefix,
// so a route reads the way the API reference writes it.
type call struct {
	Method string
	Path   string
}

// Named for the HTTP verb so a route map reads like the API reference. Upper
// case keeps them clear of anything the package itself might come to declare.
func GET(path string) call    { return call{http.MethodGet, path} }
func POST(path string) call   { return call{http.MethodPost, path} }
func PUT(path string) call    { return call{http.MethodPut, path} }
func PATCH(path string) call  { return call{http.MethodPatch, path} }
func DELETE(path string) call { return call{http.MethodDelete, path} }

// received is one request the stub answered, kept so a test can assert on what
// the command actually sent rather than only on what it printed.
type received struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
}

// decode reads the recorded request body into v.
func (r received) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("request body to %s %s is not JSON: %v\n%s", r.Method, r.Path, err, r.Body)
	}
}

// ltm is a stub loadTestManager. Routes are exact method+path matches; anything
// unrouted fails the test loudly, because a command quietly calling an endpoint
// nobody stubbed is the bug most worth catching.
type ltm struct {
	t      *testing.T
	server *httptest.Server

	mu   sync.Mutex
	seen []received
}

// serveLTM starts the stub and points the global configuration at it, so a
// command built by its real constructor talks to the stub instead of the
// network. A route value is written verbatim if it is a string or []byte,
// called if it is a http.HandlerFunc, and JSON-encoded otherwise.
func serveLTM(t *testing.T, routes map[call]any) *ltm {
	t.Helper()

	stub := &ltm{t: t}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAllAndClose(r)
		path := strings.TrimPrefix(r.URL.Path, api.APIPath)

		stub.mu.Lock()
		stub.seen = append(stub.seen, received{
			Method: r.Method,
			Path:   path,
			Query:  r.URL.Query(),
			Body:   body,
		})
		stub.mu.Unlock()

		route, ok := routes[call{r.Method, path}]
		if !ok {
			// Answering 404 would let the command report a plausible-looking
			// "not found" and the test would pass for the wrong reason.
			stub.t.Errorf("no stub for %s %s", r.Method, path)
			http.Error(w, "unrouted", http.StatusNotImplemented)
			return
		}

		switch answer := route.(type) {
		case http.HandlerFunc:
			answer(w, r)
		case string:
			w.Write([]byte(answer))
		case []byte:
			w.Write(answer)
		default:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(answer); err != nil {
				stub.t.Errorf("encoding stub response for %s %s: %v", r.Method, path, err)
			}
		}
	}))
	t.Cleanup(stub.server.Close)

	useConfig(t, config.GlobalFlags{
		APIBaseURL: stub.server.URL,
		AuthToken:  "pat.test.test.test",
		AccountID:  "acct1",
		OrgID:      "default",
		ProjectID:  "perf",
		Format:     formatJSON,
	})

	return stub
}

// writeJSON is for a route that has to vary its answer between calls, and so is
// written as a handler rather than as a value the stub encodes for it.
func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func readAllAndClose(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

// requests returns every request the stub answered, in order.
func (l *ltm) requests() []received {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]received(nil), l.seen...)
}

// only returns the single request made, failing if there was more than one.
// Most commands are one call, and asserting that is part of the point: a get
// that quietly makes two round trips is worth knowing about.
func (l *ltm) only() received {
	l.t.Helper()
	seen := l.requests()
	if len(seen) != 1 {
		l.t.Fatalf("expected exactly one request, got %d: %v", len(seen), summarise(seen))
	}
	return seen[0]
}

// find returns the first request matching method and path.
func (l *ltm) find(c call) received {
	l.t.Helper()
	for _, r := range l.requests() {
		if r.Method == c.Method && r.Path == c.Path {
			return r
		}
	}
	l.t.Fatalf("no request to %s %s; saw %v", c.Method, c.Path, summarise(l.requests()))
	return received{}
}

func summarise(seen []received) []string {
	out := make([]string, 0, len(seen))
	for _, r := range seen {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// globals exposes the live configuration so a test can vary one field — most
// often the output format — after serveLTM has established the rest. serveLTM
// has already arranged for it to be restored.
func globals() *config.GlobalFlags { return &config.Global }

// useConfig swaps the global configuration for the duration of one test. It is
// a package-level global, so it has to be restored or the next test inherits it.
func useConfig(t *testing.T, cfg config.GlobalFlags) {
	t.Helper()
	previous := config.Global
	config.Global = cfg
	t.Cleanup(func() { config.Global = previous })
}

// configFile writes body as the JSON a --config flag reads, and returns its
// path.
func configFile(t *testing.T, body map[string]any) string {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("building the config file: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}
	return path
}

// aLoadTest is a load test as the service returns one. Several commands read
// the stored definition before sending their own request — update merges into
// it, script mode is decided by it — so the fixture carries a populated tool
// config rather than an empty one.
func aLoadTest(identity string) map[string]any {
	return map[string]any{
		"uniqueId":               "1a2b3c4d-0000-4000-8000-000000000001",
		"identity":               identity,
		"name":                   "Checkout load",
		"toolType":               "K6",
		"accountIdentifier":      "acct1",
		"organizationIdentifier": "default",
		"projectIdentifier":      "perf",
		"infraIdentifier":        "perf-cluster",
		"environmentIdentifier":  "staging",
		"targetType":             "kubernetes",
		"targetUsers":            "100",
		"durationSeconds":        "600",
		"cleanupPolicy":          "delete",
		"createdAt":              "2026-07-01T09:00:00Z",
		"updatedAt":              "2026-08-01T09:00:00Z",
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "500m", "memory": "512Mi"},
			"limits":   map[string]any{"cpu": "2", "memory": "2Gi"},
		},
		"toolConfig": map[string]any{
			"k6": map[string]any{
				"script":   map[string]any{"content": "ZXhwb3J0IGRlZmF1bHQ=", "source": "inline"},
				"tunables": map[string]any{"targetUsers": 100, "durationSeconds": 600},
			},
		},
		"variables": []any{
			map[string]any{"name": "REGION", "value": "eu-west-1", "type": "String"},
		},
	}
}

// output is what a command wrote and how it ended.
type output struct {
	stdout string
	stderr string
	err    error
}

// combined is for assertions that do not care which stream a line went to.
func (o output) combined() string { return o.stdout + o.stderr }

// execute drives a command as the CLI would: real constructor, real flag
// parsing, real RunE. args are what the user would type after the command name.
func execute(t *testing.T, cmd *cobra.Command, args ...string) output {
	t.Helper()

	// pterm writes escape codes when it thinks it has a terminal, which would
	// make every table assertion depend on the environment.
	pterm.DisableColor()
	t.Cleanup(pterm.EnableColor)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.ExecuteContext(context.Background())

	return output{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// expectFailure runs a command that should not succeed and returns the message.
func expectFailure(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	result := execute(t, cmd, args...)
	if result.err == nil {
		t.Fatalf("expected %s to fail, it printed:\n%s", cmd.Name(), result.combined())
	}
	return result.err.Error()
}

// expectSuccess runs a command that should succeed and returns what it printed.
func expectSuccess(t *testing.T, cmd *cobra.Command, args ...string) output {
	t.Helper()
	result := execute(t, cmd, args...)
	if result.err != nil {
		t.Fatalf("%s failed unexpectedly: %v\n%s", cmd.Name(), result.err, result.combined())
	}
	return result
}

// mustContain asserts on rendered output with a message that shows what was
// actually printed, since a diff of the whole payload is what you need here.
func mustContain(t *testing.T, what, where string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(where, want) {
			t.Errorf("%s is missing %q:\n%s", what, want, where)
		}
	}
}

func mustNotContain(t *testing.T, what, where string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(where, bad) {
			t.Errorf("%s should not contain %q:\n%s", what, bad, where)
		}
	}
}
