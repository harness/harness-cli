package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A client method is a route and a shape, and both are load-bearing: the wrong
// verb or a path assembled from the wrong field fails only against the live
// service, which is the slowest place to find out. These tests call every
// method against a recording server and assert what went out.

// recorder answers any request with a fixed JSON body and keeps what it saw.
type recorder struct {
	server *httptest.Server

	method string
	path   string
	query  url.Values
	body   []byte
}

// record starts a server answering every route with answer, which is encoded as
// JSON unless it is already a string or []byte.
func record(t *testing.T, answer any) (*recorder, *Client) {
	t.Helper()

	rec := &recorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		if r.Body != nil {
			defer r.Body.Close()
			_, _ = buf.ReadFrom(r.Body)
		}

		rec.method = r.Method
		rec.path = strings.TrimPrefix(r.URL.Path, APIPath)
		rec.query = r.URL.Query()
		rec.body = buf.Bytes()

		switch typed := answer.(type) {
		case nil:
			w.WriteHeader(http.StatusOK)
		case string:
			_, _ = w.Write([]byte(typed))
		case []byte:
			_, _ = w.Write(typed)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(typed)
		}
	}))
	t.Cleanup(rec.server.Close)

	return rec, NewClient(Config{
		Server: rec.server.URL,
		Scope:  Scope{AccountID: "acct1", OrgID: "default", ProjectID: "perf"},
	})
}

// wants asserts the verb and path of the request the method made.
func (r *recorder) wants(t *testing.T, method, path string) {
	t.Helper()
	if r.method != method || r.path != path {
		t.Errorf("called %s %s, want %s %s", r.method, r.path, method, path)
	}
}

// Every route is scoped, and a request that loses the scope reads as an empty
// account rather than as an error, so this is checked once for all of them.
func TestEveryRequestCarriesTheScope(t *testing.T) {
	rec, client := record(t, LoadTest{Identity: "checkout-load"})

	if _, err := client.Get(context.Background(), "checkout-load"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	for key, want := range map[string]string{
		"accountIdentifier":      "acct1",
		"organizationIdentifier": "default",
		"projectIdentifier":      "perf",
	} {
		if got := rec.query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// An identity is a path segment, so one holding a slash or a space has to be
// escaped or it silently addresses a different route.
func TestAnIdentityIsEscapedIntoThePath(t *testing.T) {
	rec, client := record(t, LoadTest{})

	if _, err := client.Get(context.Background(), "checkout load/eu"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	rec.wants(t, http.MethodGet, "/load-tests/checkout load/eu")
}

func TestLoadTestRoutes(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		answer any
	}{
		{
			name:   "create",
			call:   func(c *Client) error { _, err := c.Create(ctx, CreateRequest{Identity: "checkout-load"}); return err },
			method: http.MethodPost, path: "/load-tests",
		},
		{
			name:   "list",
			call:   func(c *Client) error { _, err := c.List(ctx, ListOptions{}); return err },
			method: http.MethodGet, path: "/load-tests",
		},
		{
			name:   "get",
			call:   func(c *Client) error { _, err := c.Get(ctx, "checkout-load"); return err },
			method: http.MethodGet, path: "/load-tests/checkout-load",
		},
		{
			name:   "update",
			call:   func(c *Client) error { _, err := c.Update(ctx, "checkout-load", UpdateRequest{}); return err },
			method: http.MethodPut, path: "/load-tests/checkout-load",
		},
		{
			name:   "delete",
			call:   func(c *Client) error { return c.Delete(ctx, "checkout-load") },
			method: http.MethodDelete, path: "/load-tests/checkout-load",
		},
		{
			name:   "variables",
			call:   func(c *Client) error { _, err := c.GetVariables(ctx, "checkout-load"); return err },
			method: http.MethodGet, path: "/load-tests/checkout-load/variables",
		},
		{
			name:   "get script",
			call:   func(c *Client) error { _, err := c.GetScript(ctx, "checkout-load"); return err },
			method: http.MethodGet, path: "/load-tests/checkout-load/script",
		},
		{
			name: "update script",
			call: func(c *Client) error {
				_, err := c.UpdateScript(ctx, "checkout-load", UpdateScriptRequest{ScriptContent: "aGk="})
				return err
			},
			method: http.MethodPut, path: "/load-tests/checkout-load/script",
		},
		{
			name:   "list script revisions",
			call:   func(c *Client) error { _, err := c.ListScriptRevisions(ctx, "checkout-load"); return err },
			method: http.MethodGet, path: "/load-tests/checkout-load/script/revisions",
			answer: []any{},
		},
		{
			name: "create from json",
			call: func(c *Client) error {
				_, err := c.CreateFromJSON(ctx, CreateFromJSONRequest{Identity: "api-journey"})
				return err
			},
			method: http.MethodPost, path: "/load-tests/from-json",
		},
		{
			name: "update json spec",
			call: func(c *Client) error {
				_, err := c.UpdateJSONSpec(ctx, "api-journey", UpdateJSONSpecRequest{})
				return err
			},
			method: http.MethodPut, path: "/load-tests/api-journey/json-script",
		},
		{
			name: "create from template",
			call: func(c *Client) error {
				_, err := c.CreateFromTemplate(ctx, CreateFromTemplateRequest{Identity: "checkout-load"})
				return err
			},
			method: http.MethodPost, path: "/load-tests/from-template",
		},
		{
			name:   "sync template",
			call:   func(c *Client) error { _, err := c.SyncTemplate(ctx, "checkout-load"); return err },
			method: http.MethodPost, path: "/load-tests/checkout-load/sync-template",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, client := record(t, orObject(tc.answer, "checkout-load"))
			if err := tc.call(client); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			rec.wants(t, tc.method, tc.path)
		})
	}
}

// orObject supplies a response shaped like whatever the method decodes into: an
// object for most routes, an array for the few that return a bare list.
func orObject(answer any, identity string) any {
	if answer != nil {
		return answer
	}
	return map[string]any{"identity": identity}
}

func TestRunRoutes(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
	}{
		{
			name:   "list the runs of one test",
			call:   func(c *Client) error { _, err := c.ListRuns(ctx, "checkout-load", ListOptions{}); return err },
			method: http.MethodGet, path: "/load-tests/checkout-load/runs",
		},
		{
			name:   "list runs across the scope",
			call:   func(c *Client) error { _, err := c.ListAllRuns(ctx, ListOptions{}); return err },
			method: http.MethodGet, path: "/runs",
		},
		{
			name:   "get",
			call:   func(c *Client) error { _, err := c.GetRun(ctx, "checkout-load-a1b"); return err },
			method: http.MethodGet, path: "/runs/checkout-load-a1b",
		},
		{
			name:   "summary",
			call:   func(c *Client) error { _, err := c.GetRunSummary(ctx, "checkout-load-a1b"); return err },
			method: http.MethodGet, path: "/runs/checkout-load-a1b/summary",
		},
		{
			name:   "graph",
			call:   func(c *Client) error { _, err := c.GetRunGraph(ctx, "checkout-load-a1b", TimeRange{}); return err },
			method: http.MethodGet, path: "/runs/checkout-load-a1b/graph",
		},
		{
			name: "metrics",
			call: func(c *Client) error {
				_, err := c.GetMetrics(ctx, "checkout-load-a1b", MetricsTimeseries, TimeRange{})
				return err
			},
			method: http.MethodGet, path: "/runs/checkout-load-a1b/metrics/timeseries",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, client := record(t, map[string]any{"identity": "checkout-load-a1b"})
			if err := tc.call(client); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			rec.wants(t, tc.method, tc.path)
		})
	}
}

// The metrics view is a path segment rather than a query parameter, so an
// unrecognised one would 404 rather than fall back to a default.
func TestEveryMetricsViewIsItsOwnRoute(t *testing.T) {
	for _, view := range MetricsViews {
		t.Run(string(view), func(t *testing.T) {
			rec, client := record(t, map[string]any{})
			if _, err := client.GetMetrics(context.Background(), "checkout-load-a1b", view, TimeRange{}); err != nil {
				t.Fatalf("GetMetrics: %v", err)
			}
			rec.wants(t, http.MethodGet, "/runs/checkout-load-a1b/metrics/"+string(view))
		})
	}
}

// A window narrows a graph or metrics query. Left empty it must not appear at
// all, since an empty bound is not the same as no bound.
func TestATimeRangeIsSentOnlyWhereItWasGiven(t *testing.T) {
	rec, client := record(t, map[string]any{})
	if _, err := client.GetRunGraph(context.Background(), "checkout-load-a1b",
		TimeRange{From: "2026-08-01T09:00:00Z", To: "2026-08-01T10:00:00Z"}); err != nil {
		t.Fatalf("GetRunGraph: %v", err)
	}
	if rec.query.Get("from") == "" || rec.query.Get("to") == "" {
		t.Errorf("the window was dropped: %v", rec.query)
	}

	rec, client = record(t, map[string]any{})
	if _, err := client.GetRunGraph(context.Background(), "checkout-load-a1b", TimeRange{}); err != nil {
		t.Fatalf("GetRunGraph: %v", err)
	}
	if rec.query.Has("from") || rec.query.Has("to") {
		t.Errorf("an empty window was sent as empty bounds: %v", rec.query)
	}
}

// Stop and update only acknowledge the request, so each reads the run back. A
// caller printing what they returned would otherwise show empty columns.
func TestStopAndUpdateReadTheRunBack(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Client) (*Run, error)
		path string
	}{
		{
			name: "stop",
			call: func(c *Client) (*Run, error) { return c.StopRun(context.Background(), "checkout-load-a1b") },
			path: "/runs/checkout-load-a1b/stop",
		},
		{
			name: "update",
			call: func(c *Client) (*Run, error) {
				return c.UpdateRun(context.Background(), "checkout-load-a1b", UpdateRunRequest{})
			},
			path: "/runs/checkout-load-a1b/update",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, strings.TrimPrefix(r.URL.Path, APIPath))
				if strings.HasSuffix(r.URL.Path, tc.path) {
					_ = json.NewEncoder(w).Encode(Acknowledgement{Success: true})
					return
				}
				_ = json.NewEncoder(w).Encode(Run{Identity: "checkout-load-a1b", Status: RunStopped})
			}))
			defer server.Close()

			run, err := tc.call(NewClient(Config{Server: server.URL}))
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if run.Status != RunStopped {
				t.Errorf("status = %q, want the run read back rather than the acknowledgement", run.Status)
			}
			if len(paths) != 2 || paths[0] != tc.path || paths[1] != "/runs/checkout-load-a1b" {
				t.Errorf("called %v, want the action then a read", paths)
			}
		})
	}
}

// A stop that the service refuses must not be followed by a read that makes it
// look as though it worked.
func TestAStopThatFailsIsNotFollowedByARead(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"message":"run has already finished"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	if _, err := NewClient(Config{Server: server.URL}).StopRun(context.Background(), "checkout-load-a1b"); err == nil {
		t.Fatal("expected a refused stop to fail")
	}
	if calls != 1 {
		t.Errorf("made %d calls, want the read abandoned after the stop failed", calls)
	}
}

func TestTemplateRoutes(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		answer any
	}{
		{
			name: "create",
			call: func(c *Client) error {
				_, err := c.CreateTemplate(ctx, "harness-hub", CreateTemplateRequest{})
				return err
			},
			method: http.MethodPost, path: "/load-test-templates",
		},
		{
			name:   "list",
			call:   func(c *Client) error { _, err := c.ListTemplates(ctx, TemplateListOptions{}); return err },
			method: http.MethodGet, path: "/load-test-templates",
		},
		{
			name:   "get",
			call:   func(c *Client) error { _, err := c.GetTemplate(ctx, "harness-hub", "standard-http"); return err },
			method: http.MethodGet, path: "/load-test-templates/standard-http",
		},
		{
			name: "update",
			call: func(c *Client) error {
				_, err := c.UpdateTemplate(ctx, "harness-hub", "standard-http", UpdateTemplateRequest{})
				return err
			},
			method: http.MethodPut, path: "/load-test-templates/standard-http",
		},
		{
			name:   "delete",
			call:   func(c *Client) error { return c.DeleteTemplate(ctx, "harness-hub", "standard-http") },
			method: http.MethodDelete, path: "/load-test-templates/standard-http",
		},
		{
			name: "list revisions",
			call: func(c *Client) error {
				_, err := c.ListTemplateRevisions(ctx, "harness-hub", "standard-http")
				return err
			},
			method: http.MethodGet, path: "/load-test-templates/standard-http/revisions",
			answer: []any{},
		},
		{
			name: "get revision",
			call: func(c *Client) error {
				_, err := c.GetTemplateRevision(ctx, "harness-hub", "standard-http", "v3")
				return err
			},
			method: http.MethodGet, path: "/load-test-templates/standard-http/revisions/v3",
		},
		{
			name: "create revision",
			call: func(c *Client) error {
				_, err := c.CreateTemplateRevision(ctx, "harness-hub", "standard-http", CreateRevisionRequest{})
				return err
			},
			method: http.MethodPost, path: "/load-test-templates/standard-http/revisions",
		},
		{
			name:   "delete revision",
			call:   func(c *Client) error { return c.DeleteTemplateRevision(ctx, "harness-hub", "standard-http", "v3") },
			method: http.MethodDelete, path: "/load-test-templates/standard-http/revisions/v3",
		},
		{
			name: "variables",
			call: func(c *Client) error {
				_, err := c.GetTemplateVariables(ctx, "harness-hub", "standard-http", "")
				return err
			},
			method: http.MethodGet, path: "/load-test-templates/standard-http/variables",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, client := record(t, orObject(tc.answer, "standard-http"))
			if err := tc.call(client); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			rec.wants(t, tc.method, tc.path)

			// These endpoints match hubIdentity exactly, so it has to travel on
			// every one of them or the template reads as missing.
			if got := rec.query.Get("hubIdentity"); got != "harness-hub" && tc.name != "list" {
				t.Errorf("hubIdentity = %q, want it sent with every template call", got)
			}
		})
	}
}

// The template is YAML, which is not JSON, so it comes back undecoded rather
// than through the decoder every other route uses.
func TestExportTemplateYAMLReturnsTheBodyUntouched(t *testing.T) {
	const yaml = "loadTest:\n  identity: standard-http\n"

	rec, client := record(t, yaml)
	body, err := client.ExportTemplateYAML(context.Background(), "harness-hub", "standard-http")
	if err != nil {
		t.Fatalf("ExportTemplateYAML: %v", err)
	}

	rec.wants(t, http.MethodGet, "/load-test-templates/standard-http/yaml")
	if string(body) != yaml {
		t.Errorf("returned %q, want the YAML as it stands", body)
	}
}

// An empty revision means the latest, which is not the same as asking for a
// revision named "": one resolves, the other would not match anything.
func TestTemplateVariablesAsksForARevisionOnlyWhenGivenOne(t *testing.T) {
	rec, client := record(t, map[string]any{})
	if _, err := client.GetTemplateVariables(context.Background(), "harness-hub", "standard-http", "v3"); err != nil {
		t.Fatalf("GetTemplateVariables: %v", err)
	}
	if got := rec.query.Get("revision"); got != "v3" {
		t.Errorf("revision = %q, want the one asked for", got)
	}

	rec, client = record(t, map[string]any{})
	if _, err := client.GetTemplateVariables(context.Background(), "harness-hub", "standard-http", ""); err != nil {
		t.Fatalf("GetTemplateVariables: %v", err)
	}
	if rec.query.Has("revision") {
		t.Errorf("an empty revision was sent as a filter: %v", rec.query)
	}
}

// An empty hub selects templates filed under no hub, so it is sent as an empty
// value rather than omitted — except on the list, where it is a filter and an
// empty one would hide everything.
func TestAnEmptyHubIsStillSentOnTheRoutesThatMatchItExactly(t *testing.T) {
	rec, client := record(t, map[string]any{})
	if _, err := client.GetTemplate(context.Background(), "", "standard-http"); err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if !rec.query.Has("hubIdentity") {
		t.Errorf("hubIdentity was omitted rather than sent empty: %v", rec.query)
	}
}

func TestCompositeAndUsageRoutes(t *testing.T) {
	ctx := context.Background()
	window := UsageWindow{StartMillis: 1753952400000, EndMillis: 1754038800000}

	for _, tc := range []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		answer any
	}{
		{
			name:   "create composite",
			call:   func(c *Client) error { _, err := c.CreateComposite(ctx, CreateCompositeRequest{}); return err },
			method: http.MethodPost, path: "/composite-load-tests",
		},
		{
			name:   "list composites",
			call:   func(c *Client) error { _, err := c.ListComposites(ctx, ListOptions{}); return err },
			method: http.MethodGet, path: "/composite-load-tests",
		},
		{
			name:   "usage details",
			call:   func(c *Client) error { _, err := c.GetUsageDetails(ctx, window); return err },
			method: http.MethodGet, path: "/load-service-usage/details",
		},
		{
			name:   "overall usage",
			call:   func(c *Client) error { _, err := c.GetOverallUsage(ctx, window); return err },
			method: http.MethodGet, path: "/load-service-usage/overall",
		},
		{
			name:   "usage report",
			call:   func(c *Client) error { _, err := c.GetUsageReport(ctx, window); return err },
			method: http.MethodGet, path: "/load-service-usage/report",
			// The report is a matrix rather than an object, so it can be
			// written straight out as CSV.
			answer: [][]string{{"service", "runs"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, client := record(t, orObject(tc.answer, "checkout_resilience"))
			if err := tc.call(client); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			rec.wants(t, tc.method, tc.path)
		})
	}
}

// Usage is reported over a window and both ends are required, so neither may be
// dropped on the way out.
func TestAUsageWindowTravelsAsUnixMillis(t *testing.T) {
	rec, client := record(t, map[string]any{})

	if _, err := client.GetOverallUsage(context.Background(),
		UsageWindow{StartMillis: 1753952400000, EndMillis: 1754038800000}); err != nil {
		t.Fatalf("GetOverallUsage: %v", err)
	}

	if rec.query.Get("startTime") != "1753952400000" || rec.query.Get("endTime") != "1754038800000" {
		t.Errorf("window = %v, want both ends as unix millis", rec.query)
	}
}

// The composite listing is backed by pipeline service, which has none of these
// filters. Sending them would be accepted and ignored, so the result would read
// as filtered when it is not.
func TestTheCompositeListingDropsTheFiltersPipelineServiceIgnores(t *testing.T) {
	rec, client := record(t, CompositeList{})

	if _, err := client.ListComposites(context.Background(), ListOptions{
		Search:                "checkout",
		ToolType:              "K6",
		EnvironmentIdentifier: "staging",
		Status:                "Finished",
		Tags:                  []string{"nightly"},
	}); err != nil {
		t.Fatalf("ListComposites: %v", err)
	}

	for _, unsupported := range []string{"toolType", "environmentIdentifier", "status", "tags"} {
		if rec.query.Has(unsupported) {
			t.Errorf("%s reached the composite listing, which ignores it: %v", unsupported, rec.query)
		}
	}
	if rec.query.Get("search") != "checkout" {
		t.Errorf("a filter it does support was dropped: %v", rec.query)
	}
}

// The template store filters on hub, tool and infra type only. The rest belong
// to the load test listing and would be silently ignored here.
func TestTheTemplateListingSendsOnlyTheFiltersItHas(t *testing.T) {
	rec, client := record(t, TemplateList{})

	if _, err := client.ListTemplates(context.Background(), TemplateListOptions{
		HubIdentity: "harness-hub",
		InfraType:   "kubernetes",
		ListOptions: ListOptions{
			Search:                "http",
			ToolType:              "K6",
			EnvironmentIdentifier: "staging",
			Status:                "Finished",
			Tags:                  []string{"nightly"},
		},
	}); err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}

	for key, want := range map[string]string{
		"hubIdentity": "harness-hub",
		"infraType":   "kubernetes",
		"search":      "http",
		"toolType":    "K6",
	} {
		if got := rec.query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for _, unsupported := range []string{"environmentIdentifier", "status", "tags"} {
		if rec.query.Has(unsupported) {
			t.Errorf("%s reached the template listing, which ignores it: %v", unsupported, rec.query)
		}
	}
}

// Sorting takes a direction as well as a field, and the direction is only
// meaningful alongside one, so it is sent only when a field was given.
func TestListOptionsSendOnlyWhatWasSet(t *testing.T) {
	rec, client := record(t, LoadTestList{})

	if _, err := client.List(context.Background(), ListOptions{
		Page: 2, Limit: 50, Search: "checkout",
		SortField: "createdAt", SortAscending: true,
		EnvironmentIdentifier: "staging", ToolType: "K6",
		Tags: []string{"nightly", "eu"},
	}); err != nil {
		t.Fatalf("List: %v", err)
	}

	for key, want := range map[string]string{
		"page": "2", "limit": "50", "search": "checkout",
		"sortField": "createdAt", "sortAscending": "true",
		"environmentIdentifier": "staging", "toolType": "K6",
	} {
		if got := rec.query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	// Repeated rather than joined: a tag may hold a comma.
	if tags := rec.query["tags"]; len(tags) != 2 {
		t.Errorf("tags = %v, want each sent as its own parameter", tags)
	}

	rec, client = record(t, LoadTestList{})
	if _, err := client.List(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, unset := range []string{"page", "limit", "search", "sortField", "sortAscending", "toolType", "status", "tags"} {
		if rec.query.Has(unset) {
			t.Errorf("%s was sent though it was never set: %v", unset, rec.query)
		}
	}
}

// A run's status decides whether the watch loop keeps polling, so a status
// wrongly called terminal would end the watch early and one wrongly called
// live would hang.
func TestOnlyAFinishedRunIsTerminal(t *testing.T) {
	for status, want := range map[RunStatus]bool{
		RunStopped:  true,
		RunFinished: true,
		RunFailed:   true,
		RunRunning:  false,
		RunPending:  false,
		RunStopping: false,
		"":          false,
	} {
		if got := status.Terminal(); got != want {
			t.Errorf("%q.Terminal() = %v, want %v", status, got, want)
		}
	}
}

// A 404 is the one status a caller acts on differently: it means the object is
// not there, rather than that the call failed.
func TestIsNotFoundRecognisesOnlyA404(t *testing.T) {
	if !IsNotFound(&APIError{StatusCode: http.StatusNotFound}) {
		t.Error("a 404 was not recognised")
	}
	for _, err := range []error{
		&APIError{StatusCode: http.StatusBadRequest},
		&APIError{StatusCode: http.StatusInternalServerError},
		context.Canceled,
	} {
		if IsNotFound(err) {
			t.Errorf("%v was reported as a not-found", err)
		}
	}
}
