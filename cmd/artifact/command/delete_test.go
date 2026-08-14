package command

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withStdin temporarily swaps the package-level stdinReader.
func withStdin(t *testing.T, r io.Reader) {
	t.Helper()
	orig := stdinReader
	stdinReader = r
	t.Cleanup(func() { stdinReader = orig })
}

// withTerminal forces the stdinIsTerminal check to the given value.
func withTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return isTTY }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

// withGlobalConfig temporarily sets config.Global fields and restores them.
func withGlobalConfig(t *testing.T, account, org, project string) {
	t.Helper()
	origAcct := config.Global.AccountID
	origOrg := config.Global.OrgID
	origProj := config.Global.ProjectID
	config.Global.AccountID = account
	config.Global.OrgID = org
	config.Global.ProjectID = project
	t.Cleanup(func() {
		config.Global.AccountID = origAcct
		config.Global.OrgID = origOrg
		config.Global.ProjectID = origProj
	})
}

// withFormat temporarily sets config.Global.Format.
func withFormat(t *testing.T, format string) {
	t.Helper()
	orig := config.Global.Format
	config.Global.Format = format
	t.Cleanup(func() { config.Global.Format = orig })
}

// newTestFactory wires a Factory whose RegistryV3HttpClient hits the given
// httptest server. This naturally exercises executeBulkDelete's real HTTP path.
func newTestFactory(t *testing.T, ts *httptest.Server) *cmdutils.Factory {
	t.Helper()
	client, err := ar_v3.NewClientWithResponses(ts.URL)
	require.NoError(t, err)
	return &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}
}

// newDeleteFlowFactory wires both the v3 (bulk delete) and v1 (post-delete
// verification reads) clients of a Factory to the same base URL.
func newDeleteFlowFactory(t *testing.T, baseURL string) *cmdutils.Factory {
	t.Helper()
	v3Client, err := ar_v3.NewClientWithResponses(baseURL)
	require.NoError(t, err)
	v1Client, err := ar.NewClientWithResponses(baseURL)
	require.NoError(t, err)
	return &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return v3Client },
		RegistryHttpClient:   func() *ar.ClientWithResponses { return v1Client },
	}
}

// scriptedServer returns an httptest.Server whose successive responses are
// taken from the provided slice. Each response is (status, body).
type scriptedResponse struct {
	status int
	body   string
}

func scriptedServer(t *testing.T, responses []scriptedResponse) *httptest.Server {
	t.Helper()
	var idx int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(&idx, 1)) - 1
		if i >= len(responses) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"unexpected extra request"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(responses[i].status)
		_, _ = w.Write([]byte(responses[i].body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------
// deleteFlowServer: records every request and routes the two endpoints the
// delete flow uses - POST /bulkdelete (v3) and GET .../summary (v1 verify).
// ---------------------------------------------------------------------

type recordedDeleteRequest struct {
	Method string
	Path   string
	Body   string
}

type deleteFlowServer struct {
	srv             *httptest.Server
	mu              sync.Mutex
	requests        []recordedDeleteRequest
	deleteStatus    int
	deleteBody      string
	existenceStatus int
	existenceBody   string
}

func newDeleteFlowServer(t *testing.T, deleteStatus int, deleteBody string, existenceStatus int, existenceBody string) *deleteFlowServer {
	t.Helper()
	d := &deleteFlowServer{
		deleteStatus:    deleteStatus,
		deleteBody:      deleteBody,
		existenceStatus: existenceStatus,
		existenceBody:   existenceBody,
	}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		d.mu.Lock()
		d.requests = append(d.requests, recordedDeleteRequest{Method: r.Method, Path: r.URL.Path, Body: string(body)})
		d.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/bulkdelete"):
			w.WriteHeader(d.deleteStatus)
			_, _ = w.Write([]byte(d.deleteBody))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/summary"):
			w.WriteHeader(d.existenceStatus)
			_, _ = w.Write([]byte(d.existenceBody))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
		}
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *deleteFlowServer) requestCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.requests)
}

func (d *deleteFlowServer) firstRequest() recordedDeleteRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.requests[0]
}

// bulkDeleteRequests returns only the recorded POST /bulkdelete calls.
func (d *deleteFlowServer) bulkDeleteRequests() []recordedDeleteRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []recordedDeleteRequest
	for _, r := range d.requests {
		if r.Method == http.MethodPost && strings.Contains(r.Path, "/bulkdelete") {
			out = append(out, r)
		}
	}
	return out
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string)
	go func() {
		out, _ := io.ReadAll(r)
		done <- string(out)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func newDeleteCmd(factory *cmdutils.Factory, args ...string) interface{ Execute() error } {
	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// ---------------------------------------------------------------------
// printOutPut
// ---------------------------------------------------------------------

func TestPrintOutPut(t *testing.T) {
	assert.NoError(t, printOutPut(nil))
	assert.NoError(t, printOutPut([]string{"pkg-a@1.0.0", "pkg-b@2.0.0"}))
}

// ---------------------------------------------------------------------
// splitCoordinate
// ---------------------------------------------------------------------

func TestSplitCoordinate(t *testing.T) {
	cases := []struct {
		in      string
		name    string
		version string
	}{
		{"pkg@1.0.0", "pkg", "1.0.0"},
		{"pkg", "pkg", ""},
		{"@scope/name@1.0.0", "@scope/name", "1.0.0"},
		{"@scope/name", "@scope/name", ""},
		{"group:artifact@2.1", "group:artifact", "2.1"},
		{"@", "@", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			name, version := splitCoordinate(tc.in)
			assert.Equal(t, tc.name, name)
			assert.Equal(t, tc.version, version)
		})
	}
}

// ---------------------------------------------------------------------
// verifyCoordinate - direct unit tests against the v1 read API
// ---------------------------------------------------------------------

func TestVerifyCoordinate_VersionGoneIsSoftDeleted(t *testing.T) {
	d := newDeleteFlowServer(t, 200, ``, http.StatusNotFound, `{"error":{"message":"not found"}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	v := verifyCoordinate(factory, "acct/myreg", "a@1.0.0", outcomeSoftDeleted)
	assert.Equal(t, "a@1.0.0", v.Coordinate)
	assert.Equal(t, outcomeSoftDeleted, v.Outcome)
}

func TestVerifyCoordinate_PackageGoneIsHardDeletedWithForce(t *testing.T) {
	d := newDeleteFlowServer(t, 200, ``, http.StatusNotFound, `{"error":{"message":"not found"}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	v := verifyCoordinate(factory, "acct/myreg", "mypkg", outcomeHardDeleted)
	assert.Equal(t, outcomeHardDeleted, v.Outcome)
}

// The P2c defect shape: server claimed success, coordinate still resolves.
func TestVerifyCoordinate_StillPresentIsUnchanged(t *testing.T) {
	d := newDeleteFlowServer(t, 200, ``, http.StatusOK, `{"data":{}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	v := verifyCoordinate(factory, "acct/myreg", "a@1.0.0", outcomeSoftDeleted)
	assert.Equal(t, outcomeUnchanged, v.Outcome)
	assert.Contains(t, v.Detail, "still present")
}

func TestVerifyCoordinate_ReadUnsupported(t *testing.T) {
	d := newDeleteFlowServer(t, 200, ``, http.StatusMethodNotAllowed, `{"error":{"message":"method not allowed"}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	v := verifyCoordinate(factory, "acct/myreg", "a@1.0.0", outcomeSoftDeleted)
	assert.Equal(t, outcomeUnsupported, v.Outcome)
	assert.Contains(t, v.Detail, "405")
}

func TestVerifyCoordinate_UnexpectedStatusIsUnsupported(t *testing.T) {
	d := newDeleteFlowServer(t, 200, ``, http.StatusInternalServerError, `{"error":{"message":"boom"}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	v := verifyCoordinate(factory, "acct/myreg", "a@1.0.0", outcomeSoftDeleted)
	assert.Equal(t, outcomeUnsupported, v.Outcome)
	assert.Contains(t, v.Detail, "500")
}

func TestVerifyCoordinate_TransportErrorIsUnsupported(t *testing.T) {
	v1Client, err := ar.NewClientWithResponses("http://127.0.0.1:1") // unreachable
	require.NoError(t, err)
	factory := &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return v1Client },
	}

	v := verifyCoordinate(factory, "acct/myreg", "a@1.0.0", outcomeSoftDeleted)
	assert.Equal(t, outcomeUnsupported, v.Outcome)
	assert.Contains(t, v.Detail, "verification read failed")
}

func TestVerifyCoordinate_NilReadClientIsUnsupported(t *testing.T) {
	v := verifyCoordinate(&cmdutils.Factory{}, "acct/myreg", "a@1.0.0", outcomeSoftDeleted)
	assert.Equal(t, outcomeUnsupported, v.Outcome)
	assert.Contains(t, v.Detail, "unavailable")
}

// ---------------------------------------------------------------------
// §4 W5 regression tests - the dry-run trap
// ---------------------------------------------------------------------

// W5(a): piped "y" + --dry-run=true must print the preview and exit 0 with
// no follow-up mutation call. Before the fix this performed a REAL delete.
func TestDelete_DryRunPipedYesNeverMutates(t *testing.T) {
	withStdin(t, strings.NewReader("y\n")) // non-file reader => non-TTY

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--dry-run=true"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())

	// Exactly one request, and it must be the dry-run (non-mutating) call.
	require.Equal(t, 1, d.requestCount(), "dry-run must not trigger any follow-up call")
	first := d.firstRequest()
	assert.Equal(t, http.MethodPost, first.Method)
	assert.Contains(t, first.Path, "/bulkdelete")
	assert.Contains(t, first.Body, `"dryRun":true`)
}

// W5(b): closed/empty stdin + --dry-run=true must produce a clean exit-0
// preview, not an abort. An erroring reader proves stdin is never even read.
func TestDelete_DryRunClosedStdinCleanPreview(t *testing.T) {
	withStdin(t, iotest.ErrReader(io.ErrClosedPipe))

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
	assert.Equal(t, 1, d.requestCount())
}

// Even on a real terminal a dry run must never prompt or mutate.
func TestDelete_DryRunOnTTYStillNeverMutates(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	withTerminal(t, true)

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
	assert.Equal(t, 1, d.requestCount())
	assert.Contains(t, d.firstRequest().Body, `"dryRun":true`)
}

// W5(c): --dry-run=false --yes executes the real delete without prompting.
func TestDelete_RealDeleteWithYesExecutesAndVerifies(t *testing.T) {
	withStdin(t, iotest.ErrReader(io.ErrClosedPipe)) // prove stdin is never read

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		http.StatusNotFound, `{"error":{"message":"not found"}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())

	// One mutating bulk delete + one verification read.
	assert.Equal(t, 2, d.requestCount())
	first := d.firstRequest()
	assert.Contains(t, first.Path, "/bulkdelete")
	assert.Contains(t, first.Body, `"dryRun":false`)
}

// W5(d): --dry-run=false with non-TTY stdin and no --yes must fail before
// any network call, even if the pipe contains "y".
func TestDelete_RealDeleteNonTTYWithoutYesFails(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--dry-run=false"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation required")
	assert.Equal(t, 0, d.requestCount(), "no request may be sent without confirmation")
}

// W3: --yes together with --dry-run=true is a contradiction.
func TestDelete_YesWithDryRunIsContradiction(t *testing.T) {
	d := newDeleteFlowServer(t, 200, `{"dryRun": true}`, 0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--yes"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.Equal(t, 0, d.requestCount())
}

// Interactive TTY confirmation path: user answers "y".
func TestDelete_RealDeleteTTYConfirmed(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	withTerminal(t, true)

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--dry-run=false"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
	assert.Equal(t, 2, d.requestCount())
}

// Interactive TTY confirmation path: user answers "n" -> abort, no request.
func TestDelete_RealDeleteTTYCancelled(t *testing.T) {
	withStdin(t, strings.NewReader("n\n"))
	withTerminal(t, true)

	d := newDeleteFlowServer(t, 200, `{"dryRun": false}`, 0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--dry-run=false"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by user")
	assert.Equal(t, 0, d.requestCount())
}

// TTY confirmation with a broken reader surfaces the read error.
func TestDelete_RealDeleteTTYPromptReadError(t *testing.T) {
	withStdin(t, iotest.ErrReader(io.ErrUnexpectedEOF))
	withTerminal(t, true)

	cmd := newDeleteCmd(&cmdutils.Factory{}, "valid-pkg", "--registry", "myreg", "--dry-run=false")
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read confirmation")
}

// --force with --dry-run=false still requires confirmation on non-TTY stdin.
func TestDelete_ForceRealDeleteNonTTYWithoutYesFails(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))

	d := newDeleteFlowServer(t, 200, `{"dryRun": false}`, 0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--force", "--dry-run=false"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation required")
	assert.Equal(t, 0, d.requestCount())
}

// --force --dry-run=false --yes performs a hard delete (force:true) and
// verifies the coordinates are gone.
func TestDelete_ForceRealDeleteWithYes(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "force": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--force", "--dry-run=false", "--yes"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())

	deletes := d.bulkDeleteRequests()
	require.Len(t, deletes, 1)
	assert.Contains(t, deletes[0].Body, `"force":true`)
	assert.Contains(t, deletes[0].Body, `"dryRun":false`)
}

// --force with the default dry-run mode only previews; no prompt, no mutation.
func TestDelete_ForceDryRunNoPromptNoMutation(t *testing.T) {
	withStdin(t, iotest.ErrReader(io.ErrClosedPipe))

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "force": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--force"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
	assert.Equal(t, 1, d.requestCount())
	assert.Contains(t, d.firstRequest().Body, `"dryRun":true`)
}

// ---------------------------------------------------------------------
// §4 W4 - machine-readable no-mutation guarantee
// ---------------------------------------------------------------------

func TestDelete_DryRunEmitsMutatedFalseLine(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	var runErr error
	out := captureStdout(t, func() {
		runErr = newDeleteCmd(factory, "valid-pkg", "--registry", "myreg").Execute()
	})
	assert.NoError(t, runErr)
	assert.Contains(t, out, "mutated: false")
}

func TestDelete_DryRunJSONEmitsMutatedFalse(t *testing.T) {
	withFormat(t, "json")

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 2, "total": 2, "successPackages": ["a@1.0.0"], "registry": "myreg", "versionPattern": "*"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	var runErr error
	out := captureStdout(t, func() {
		runErr = newDeleteCmd(factory, "valid-pkg", "--registry", "myreg").Execute()
	})
	assert.NoError(t, runErr)
	assert.Contains(t, out, `"mutated": false`)

	// The JSON document itself must be extractable and well-formed.
	start := strings.Index(out, "{")
	require.GreaterOrEqual(t, start, 0)
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out[start:]), &doc))
	assert.Equal(t, false, doc["mutated"])
	assert.Equal(t, true, doc["dryRun"])
	assert.EqualValues(t, 1, doc["notListed"]) // success=2, one coordinate listed
}

// ---------------------------------------------------------------------
// §5 - post-delete verification / per-coordinate outcomes
// ---------------------------------------------------------------------

// §5 W1/W2: a delete that changes nothing must exit non-zero and say so.
func TestDelete_RealDeleteUnchangedCoordinateFails(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`,
		http.StatusOK, `{"data":{"name":"a"}}`) // coordinate still resolves
	factory := newDeleteFlowFactory(t, d.srv.URL)

	var runErr error
	out := captureStdout(t, func() {
		runErr = newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "UNCHANGED")
	assert.Contains(t, out, "a@1.0.0 -> UNCHANGED")
}

// Per-coordinate outcomes are printed for verified deletions.
func TestDelete_RealDeleteReportsPerCoordinateOutcomes(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 2, "total": 2, "successPackages": ["a@1.0.0", "b@2.0.0"]}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	var runErr error
	out := captureStdout(t, func() {
		runErr = newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	})
	assert.NoError(t, runErr)
	assert.Contains(t, out, "a@1.0.0 -> SOFT_DELETED")
	assert.Contains(t, out, "b@2.0.0 -> SOFT_DELETED")
	assert.Contains(t, out, "mutated: true")
}

// Force deletes report HARD_DELETED once coordinates verify gone.
func TestDelete_ForceDeleteReportsHardDeleted(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "force": true, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	var runErr error
	out := captureStdout(t, func() {
		runErr = newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--force", "--dry-run=false", "--yes").Execute()
	})
	assert.NoError(t, runErr)
	assert.Contains(t, out, "a@1.0.0 -> HARD_DELETED")
}

// §5 W3: a 405 from the bulk delete endpoint fails before any success claim
// and points at the REST restore path.
func TestDelete_BulkDelete405FailsFast(t *testing.T) {
	d := newDeleteFlowServer(t, http.StatusMethodNotAllowed,
		`{"error":{"message":"method not allowed"}}`, 0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	err := newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "405")
	assert.Contains(t, err.Error(), "restore")
	assert.Equal(t, 1, d.requestCount())
}

// Coordinates that cannot be re-read (e.g. type with no existence read)
// surface as UNSUPPORTED and fail the run instead of claiming success.
func TestDelete_VerificationUnsupportedFails(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`,
		http.StatusMethodNotAllowed, `{"error":{"message":"method not allowed"}}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	err := newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNSUPPORTED")
}

// Server-reported per-coordinate failures fail the run instead of being
// absorbed into a success line.
func TestDelete_ServerReportedFailuresExitNonZero(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "failed": 1, "total": 2, "successPackages": ["a@1.0.0"], "failedPackages": ["b@2.0.0"]}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	err := newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

// JSON output for a real delete carries mutated:true plus per-coordinate outcomes.
func TestDelete_RealDeleteJSONOutput(t *testing.T) {
	withFormat(t, "json")

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "registry": "myreg"}`,
		http.StatusNotFound, `{}`)
	factory := newDeleteFlowFactory(t, d.srv.URL)

	var runErr error
	out := captureStdout(t, func() {
		runErr = newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	})
	assert.NoError(t, runErr)
	start := strings.Index(out, "{")
	require.GreaterOrEqual(t, start, 0)
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out[start:]), &doc))
	assert.Equal(t, true, doc["mutated"])
	coords, ok := doc["coordinates"].([]interface{})
	require.True(t, ok)
	require.Len(t, coords, 1)
	coord := coords[0].(map[string]interface{})
	assert.Equal(t, "a@1.0.0", coord["coordinate"])
	assert.Equal(t, "SOFT_DELETED", coord["outcome"])
}

// Nothing matched: still a clean, truthful exit.
func TestDelete_RealDeleteNothingMatched(t *testing.T) {
	d := newDeleteFlowServer(t, 200,
		`{"dryRun": false, "success": 0, "total": 0, "successPackages": [], "message": "nothing matched"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	err := newDeleteCmd(factory, "valid-pkg", "--registry", "myreg", "--dry-run=false", "--yes").Execute()
	assert.NoError(t, err)
	assert.Equal(t, 1, d.requestCount())
}

// ---------------------------------------------------------------------
// printDryRunPreview / reportAndVerifyDelete - direct unit tests
// ---------------------------------------------------------------------

func TestPrintDryRunPreview_EmptyPreview(t *testing.T) {
	p := progress.NewConsoleReporter()
	parsed := &bulkDeleteDryRunResponse{DryRun: true, Message: "nothing to delete"}
	printDryRunPreview(parsed, "Packages", p) // must not prompt or panic
}

func TestPrintDryRunPreview_WithFailuresAndExtras(t *testing.T) {
	p := progress.NewConsoleReporter()
	parsed := &bulkDeleteDryRunResponse{
		DryRun:          true,
		Success:         5,
		Failed:          1,
		SuccessPackages: []string{"a@1.0.0", "b@1.0.0"},
		FailedPackages:  []string{"c@1.0.0"},
	}
	printDryRunPreview(parsed, "Versions", p)
}

// Invalid JSON in a 200 response is an error, not a silent success. With a
// JSON content type the generated client rejects the body first, so the error
// surfaces from executeBulkDelete rather than the manual parse in RunE.
func TestDelete_Unparseable200ResponseFails(t *testing.T) {
	d := newDeleteFlowServer(t, 200, `{not-json`, 0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	err := newDeleteCmd(factory, "valid-pkg", "--registry", "myreg").Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk delete execution")
}

// ---------------------------------------------------------------------
// executeBulkDelete via real HTTP transport
// ---------------------------------------------------------------------

func TestExecuteBulkDelete_Success(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"dryRun": true, "success": 0, "successPackages": []}`},
	})
	factory := newTestFactory(t, ts)
	p := progress.NewConsoleReporter()

	resp, err := executeBulkDelete(
		factory,
		&ar_v3.BulkDeleteArtifactsParams{},
		"art", "ver", "reg",
		false, true,
		p,
	)
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode())
}

// Bad URL forces transport error inside executeBulkDelete.
func TestExecuteBulkDelete_TransportError(t *testing.T) {
	client, err := ar_v3.NewClientWithResponses("http://127.0.0.1:1") // unreachable
	require.NoError(t, err)
	factory := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	p := progress.NewConsoleReporter()
	resp, err := executeBulkDelete(
		factory,
		&ar_v3.BulkDeleteArtifactsParams{},
		"art", "ver", "reg",
		false, true,
		p,
	)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "bulk delete execution")
}

// ---------------------------------------------------------------------
// NewDeleteArtifactCmd / cmd.Execute - end-to-end
// ---------------------------------------------------------------------

func TestNewDeleteArtifactCmd_FlagsAndUsage(t *testing.T) {
	cmd := NewDeleteArtifactCmd(&cmdutils.Factory{})
	assert.Equal(t, "delete [artifact-name]", cmd.Use)

	for _, name := range []string{"registry", "version", "force", "dry-run", "yes"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "missing flag: %s", name)
	}
	annotations := cmd.Flags().Lookup("registry").Annotations
	assert.Contains(t, annotations, "cobra_annotation_bash_completion_one_required_flag")

	// dry-run must stay opt-out: the default is the safe preview mode.
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	assert.Equal(t, "true", dryRunFlag.DefValue)
}

func TestNewDeleteArtifactCmd_InvalidArtifactPattern(t *testing.T) {
	cmd := NewDeleteArtifactCmd(&cmdutils.Factory{})
	cmd.SetArgs([]string{"ex[press", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.Error(t, cmd.Execute())
}

func TestNewDeleteArtifactCmd_InvalidVersionPattern(t *testing.T) {
	cmd := NewDeleteArtifactCmd(&cmdutils.Factory{})
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--version", "ex{bad"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.Error(t, cmd.Execute())
}

// A valid --version still flows into a dry-run preview with no prompt and
// no follow-up mutation.
func TestNewDeleteArtifactCmd_ValidVersionPatternDryRunPreview(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"], "versionPattern": "1.*"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--version", "1.*"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
	assert.Equal(t, 1, d.requestCount())
}

// Org/project config is threaded into the bulk delete params.
func TestNewDeleteArtifactCmd_DryRunWithOrgProject(t *testing.T) {
	withGlobalConfig(t, "acct-1", "org-1", "proj-1")

	d := newDeleteFlowServer(t, 200,
		`{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"], "registry": "myreg", "versionPattern": "*"}`,
		0, "")
	factory := newDeleteFlowFactory(t, d.srv.URL)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
	assert.Equal(t, 1, d.requestCount())
}

// HTTP returns non-200 with a structured error - cmd surfaces the message.
func TestNewDeleteArtifactCmd_Non200Response(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{404, `{"error":{"message":"registry not found"}}`},
	})
	factory := newTestFactory(t, ts)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registry not found")
}

// Transport error from the HTTP layer.
func TestNewDeleteArtifactCmd_TransportError(t *testing.T) {
	client, err := ar_v3.NewClientWithResponses("http://127.0.0.1:1")
	require.NoError(t, err)
	factory := &cmdutils.Factory{
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses { return client },
	}

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.Error(t, cmd.Execute())
}

// Sanity check the dry-run JSON shape the server returns is valid for the
// custom unmarshaler used in production.
func TestBulkDeleteDryRunResponse_Roundtrip(t *testing.T) {
	want := bulkDeleteDryRunResponse{
		DryRun:          true,
		Success:         2,
		SuccessPackages: []string{"a", "b"},
	}
	b, err := json.Marshal(want)
	require.NoError(t, err)

	var got bulkDeleteDryRunResponse
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, want.DryRun, got.DryRun)
	assert.Equal(t, want.Success, got.Success)
	assert.Equal(t, want.SuccessPackages, got.SuccessPackages)
}
