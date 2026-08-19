package api

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ListOptions are the filters shared by the load test and run listings. A
// zero Limit lets the API apply its own default.
type ListOptions struct {
	Page                  int64
	Limit                 int64
	Search                string
	SortField             string
	SortAscending         bool
	EnvironmentIdentifier string
	ToolType              string
	Tags                  []string
	// Status filters runs only.
	Status string
}

func (o ListOptions) values() url.Values {
	v := url.Values{}
	if o.Page > 0 {
		v.Set("page", strconv.FormatInt(o.Page, 10))
	}
	if o.Limit > 0 {
		v.Set("limit", strconv.FormatInt(o.Limit, 10))
	}
	if o.Search != "" {
		v.Set("search", o.Search)
	}
	if o.SortField != "" {
		v.Set("sortField", o.SortField)
		v.Set("sortAscending", strconv.FormatBool(o.SortAscending))
	}
	if o.EnvironmentIdentifier != "" {
		v.Set("environmentIdentifier", o.EnvironmentIdentifier)
	}
	if o.ToolType != "" {
		v.Set("toolType", o.ToolType)
	}
	if o.Status != "" {
		v.Set("status", o.Status)
	}
	for _, tag := range o.Tags {
		v.Add("tags", tag)
	}
	return v
}

func (c *Client) Create(ctx context.Context, body CreateRequest) (*LoadTest, error) {
	out := &LoadTest{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-tests",
		Body:   body,
	}, out)
	return out, err
}

func (c *Client) List(ctx context.Context, opts ListOptions) (*LoadTestList, error) {
	out := &LoadTestList{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests",
		Query:  opts.values(),
	}, out)
	return out, err
}

func (c *Client) Get(ctx context.Context, identity string) (*LoadTest, error) {
	out := &LoadTest{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests/" + url.PathEscape(identity),
	}, out)
	return out, err
}

func (c *Client) Update(ctx context.Context, identity string, body UpdateRequest) (*LoadTest, error) {
	out := &LoadTest{}
	err := c.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/load-tests/" + url.PathEscape(identity),
		Body:   body,
	}, out)
	return out, err
}

func (c *Client) Delete(ctx context.Context, identity string) error {
	return c.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/load-tests/" + url.PathEscape(identity),
	}, nil)
}

func (c *Client) GetVariables(ctx context.Context, identity string) (*VariablesResponse, error) {
	out := &VariablesResponse{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/variables",
	}, out)
	return out, err
}

// GetScript returns the current script revision of a test.
func (c *Client) GetScript(ctx context.Context, identity string) (*ScriptRevision, error) {
	out := &ScriptRevision{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/script",
	}, out)
	return out, err
}

// UpdateScript replaces the script and creates a new revision.
func (c *Client) UpdateScript(ctx context.Context, identity string, body UpdateScriptRequest) (*ScriptRevision, error) {
	out := &ScriptRevision{}
	err := c.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/script",
		Body:   body,
	}, out)
	return out, err
}

func (c *Client) ListScriptRevisions(ctx context.Context, identity string) ([]*ScriptRevision, error) {
	var out []*ScriptRevision
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/script/revisions",
	}, &out)
	return out, err
}

// GetScriptRevision returns one revision, by revision number or by identity.
// The endpoint only accepts the identity, so a number costs a listing lookup.
func (c *Client) GetScriptRevision(ctx context.Context, identity, revision string) (*ScriptRevision, error) {
	revisionID, err := c.scriptRevisionID(ctx, identity, revision)
	if err != nil {
		return nil, err
	}

	out := &ScriptRevision{}
	err = c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/script/revisions/" + url.PathEscape(revisionID),
	}, out)
	return out, err
}

func (c *Client) scriptRevisionID(ctx context.Context, identity, revision string) (string, error) {
	number, err := strconv.Atoi(revision)
	if err != nil {
		return revision, nil
	}

	revisions, err := c.ListScriptRevisions(ctx, identity)
	if err != nil {
		return "", err
	}

	available := make([]string, 0, len(revisions))
	for _, candidate := range revisions {
		if candidate.RevisionNumber == number {
			return candidate.Identity, nil
		}
		available = append(available, strconv.Itoa(candidate.RevisionNumber))
	}

	if len(available) == 0 {
		return "", fmt.Errorf("load test %q has no script revisions", identity)
	}
	return "", fmt.Errorf("load test %q has no script revision %d; it has %s",
		identity, number, strings.Join(available, ", "))
}

// CreateRun starts a run. body.Identity names the run, not the load test, and
// must be unique in scope; left empty one is generated and retried on conflict.
func (c *Client) CreateRun(ctx context.Context, identity string, body CreateRunRequest) (*Run, error) {
	if body.Identity != "" {
		return c.createRun(ctx, identity, body)
	}

	var err error
	for attempt := 0; attempt < runIdentityAttempts; attempt++ {
		body.Identity = NewRunIdentity(identity)

		var run *Run
		run, err = c.createRun(ctx, identity, body)
		if err == nil {
			return run, nil
		}
		if !IsConflict(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("could not find an unused identity for a run of %q after %d attempts: %w",
		identity, runIdentityAttempts, err)
}

func (c *Client) createRun(ctx context.Context, identity string, body CreateRunRequest) (*Run, error) {
	out := &Run{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/runs",
		Body:   body,
	}, out)
	if err != nil {
		return nil, err
	}

	// The response names the load test by its unique id; we already know the
	// identity, so restore it without the lookup namesParentByIdentity needs.
	if out.LoadTestIdentity == "" || isUniqueID(out.LoadTestIdentity) {
		out.LoadTestIdentity = identity
	}
	return out, nil
}

const (
	runIdentityAttempts = 5
	// Three characters, lowercase and digits, is what the console appends, so
	// runs started from either place sort together.
	runIdentitySuffixLen = 3
	runIdentityAlphabet  = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// NewRunIdentity names a run "<load test>-<suffix>", as the console does.
func NewRunIdentity(loadTestIdentity string) string {
	suffix := make([]byte, runIdentitySuffixLen)
	for i := range suffix {
		suffix[i] = runIdentityAlphabet[randomIndex(len(runIdentityAlphabet))]
	}
	return loadTestIdentity + "-" + string(suffix)
}

// randomIndex falls back to the clock rather than erroring: the caller retries
// collisions anyway, so a weaker suffix beats a failure it cannot act on.
func randomIndex(n int) int {
	value, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return int(time.Now().UnixNano()) % n
	}
	return int(value.Int64())
}

// ListRuns lists the runs of one load test.
func (c *Client) ListRuns(ctx context.Context, identity string, opts ListOptions) (*RunList, error) {
	out := &RunList{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/runs",
		Query:  opts.values(),
	}, out)
	return out, err
}

// ListAllRuns lists runs across every load test in scope. Set
// opts.EnvironmentIdentifier or opts.Status to narrow it.
func (c *Client) ListAllRuns(ctx context.Context, opts ListOptions) (*RunList, error) {
	out := &RunList{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/runs",
		Query:  opts.values(),
	}, out)
	return out, err
}

func (c *Client) GetRun(ctx context.Context, identity string) (*Run, error) {
	out := &Run{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/runs/" + url.PathEscape(identity),
	}, out)
	if err != nil {
		return nil, err
	}
	c.namesParentByIdentity(ctx, out)
	return out, nil
}

// namesParentByIdentity undoes the unique-id substitution GET /runs/{identity}
// makes. A failed translation is left as-is; no route accepts a unique id.
func (c *Client) namesParentByIdentity(ctx context.Context, run *Run) {
	if run == nil || !isUniqueID(run.LoadTestIdentity) {
		return
	}
	uniqueID := run.LoadTestIdentity

	c.parentMu.Lock()
	cached, seen := c.parentIdentities[uniqueID]
	c.parentMu.Unlock()
	if seen {
		if cached != "" {
			run.LoadTestIdentity = cached
		}
		return
	}

	identity := ""
	if test, err := c.FindLoadTestByUniqueID(ctx, uniqueID); err == nil && test != nil {
		identity = test.Identity
	}

	c.parentMu.Lock()
	c.parentIdentities[uniqueID] = identity
	c.parentMu.Unlock()

	if identity != "" {
		run.LoadTestIdentity = identity
	}
}

var uniqueIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUniqueID tells an internal unique id from a user-facing identity, which
// the service holds to a short slug.
func isUniqueID(value string) bool { return uniqueIDPattern.MatchString(value) }

// FindLoadTestByUniqueID pages the listing looking for a unique id, or returns
// nil. There is no lookup by unique id and the runs listing cannot be filtered.
func (c *Client) FindLoadTestByUniqueID(ctx context.Context, uniqueID string) (*LoadTest, error) {
	const pageSize = 100

	for page := int64(0); ; page++ {
		listing, err := c.List(ctx, ListOptions{Page: page, Limit: pageSize})
		if err != nil {
			return nil, err
		}
		for _, test := range listing.Items {
			if test.UniqueID == uniqueID {
				return test, nil
			}
		}
		// A short page also ends the walk, so a service that omits pagination
		// cannot spin here.
		if len(listing.Items) < pageSize || page+1 >= listing.Pagination.TotalPages {
			return nil, nil
		}
	}
}

// StopRun stops a run and returns its state afterwards. The stop endpoint
// itself only acknowledges the request; see Acknowledgement.
func (c *Client) StopRun(ctx context.Context, identity string) (*Run, error) {
	out := &Acknowledgement{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/runs/" + url.PathEscape(identity) + "/stop",
	}, out)
	if err != nil {
		return nil, err
	}
	return c.GetRun(ctx, identity)
}

// UpdateRun retunes a run in flight. The change is queued, so the run read
// back may briefly still report the previous figures.
func (c *Client) UpdateRun(ctx context.Context, identity string, body UpdateRunRequest) (*Run, error) {
	out := &Acknowledgement{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/runs/" + url.PathEscape(identity) + "/update",
		Body:   body,
	}, out)
	if err != nil {
		return nil, err
	}
	return c.GetRun(ctx, identity)
}

// Acknowledgement is what stop and update answer with. They do not echo the
// run, so decoding into a Run gives a zero value that prints as empty columns.
type Acknowledgement struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// TimeRange bounds a metrics query. Empty values mean the whole run.
type TimeRange struct {
	From string
	To   string
}

func (t TimeRange) values() url.Values {
	v := url.Values{}
	if t.From != "" {
		v.Set("from", t.From)
	}
	if t.To != "" {
		v.Set("to", t.To)
	}
	return v
}

// GetRunSummary returns the aggregate result of a finished run. The shape is
// visualization-specific, so it is returned undecoded.
func (c *Client) GetRunSummary(ctx context.Context, identity string) (map[string]any, error) {
	out := map[string]any{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/runs/" + url.PathEscape(identity) + "/summary",
	}, &out)
	return out, err
}

func (c *Client) GetRunGraph(ctx context.Context, identity string, window TimeRange) (map[string]any, error) {
	out := map[string]any{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/runs/" + url.PathEscape(identity) + "/graph",
		Query:  window.values(),
	}, &out)
	return out, err
}

// MetricsView selects which metrics projection to fetch.
type MetricsView string

const (
	MetricsTimeseries MetricsView = "timeseries"
	MetricsScatter    MetricsView = "scatter"
	MetricsAggregate  MetricsView = "aggregate"
)

var MetricsViews = []MetricsView{MetricsTimeseries, MetricsScatter, MetricsAggregate}

func (c *Client) GetMetrics(ctx context.Context, identity string, view MetricsView, window TimeRange) (map[string]any, error) {
	out := map[string]any{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/runs/" + url.PathEscape(identity) + "/metrics/" + string(view),
		Query:  window.values(),
	}, &out)
	return out, err
}
