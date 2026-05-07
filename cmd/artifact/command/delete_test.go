package command

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
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
// printOutPut
// ---------------------------------------------------------------------

func TestPrintOutPut(t *testing.T) {
	assert.NoError(t, printOutPut(nil))
	assert.NoError(t, printOutPut([]string{"pkg-a@1.0.0", "pkg-b@2.0.0"}))
}

// ---------------------------------------------------------------------
// executeWithDryRunResponse - direct unit tests
// ---------------------------------------------------------------------

func TestExecuteWithDryRunResponse_InvalidJSON(t *testing.T) {
	p := progress.NewConsoleReporter()
	err := executeWithDryRunResponse([]byte("{not-json"), p, "Packages", nil, nil, "art", "ver", "reg", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestExecuteWithDryRunResponse_NoSuccessPackages(t *testing.T) {
	p := progress.NewConsoleReporter()
	body := `{"dryRun": false, "successPackages": [], "message": "nothing to delete"}`
	err := executeWithDryRunResponse([]byte(body), p, "Packages", nil, nil, "art", "ver", "reg", false)
	assert.NoError(t, err)
}

// dryRun=false: header print + printOutPut + extra-message (real-run text) + failed-packages loop.
func TestExecuteWithDryRunResponse_RealRunSuccessesAndFailures(t *testing.T) {
	p := progress.NewConsoleReporter()
	body := `{
		"dryRun": false,
		"success": 5,
		"failed": 1,
		"total": 6,
		"successPackages": ["a@1.0.0", "b@1.0.0"],
		"failedPackages": ["c@1.0.0"],
		"registry": "myreg",
		"versionPattern": "*",
		"message": "done"
	}`
	err := executeWithDryRunResponse([]byte(body), p, "Versions", nil, nil, "art", "*", "myreg", false)
	assert.NoError(t, err)
}

// dryRun=true with extra>0: covers the "will be impacted" branch of extraMessege.
func TestExecuteWithDryRunResponse_DryRunExtraImpactedBranch(t *testing.T) {
	withStdin(t, strings.NewReader("n\n"))
	p := progress.NewConsoleReporter()
	body := `{
		"dryRun": true,
		"success": 5,
		"successPackages": ["a@1.0.0", "b@1.0.0"]
	}`
	err := executeWithDryRunResponse([]byte(body), p, "Packages", nil, nil, "art", "ver", "reg", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by user")
}

func TestExecuteWithDryRunResponse_DryRunUserCancels(t *testing.T) {
	withStdin(t, strings.NewReader("n\n"))
	p := progress.NewConsoleReporter()
	body := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	err := executeWithDryRunResponse([]byte(body), p, "Packages", nil, nil, "art", "ver", "reg", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by user")
}

func TestExecuteWithDryRunResponse_DryRunReadError(t *testing.T) {
	withStdin(t, strings.NewReader(""))
	p := progress.NewConsoleReporter()
	body := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	err := executeWithDryRunResponse([]byte(body), p, "Packages", nil, nil, "art", "ver", "reg", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read confirmation")
}

// dryRun=true + user confirms with "y" -> a real HTTP request is fired.
// The httptest server returns a real-run JSON; expect no error.
func TestExecuteWithDryRunResponse_DryRunConfirmAndExecute(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "message": "deleted"}`},
	})
	factory := newTestFactory(t, ts)

	p := progress.NewConsoleReporter()
	body := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	err := executeWithDryRunResponse(
		[]byte(body), p, "Packages",
		factory, &ar_v3.BulkDeleteArtifactsParams{},
		"art", "ver", "reg", false,
	)
	assert.NoError(t, err)
}

// dryRun=true confirmed but actual run returns non-200 with error message.
func TestExecuteWithDryRunResponse_DryRunActualRunNon200(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	ts := scriptedServer(t, []scriptedResponse{
		{404, `{"error":{"message":"registry not found"}}`},
	})
	factory := newTestFactory(t, ts)

	p := progress.NewConsoleReporter()
	body := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	err := executeWithDryRunResponse(
		[]byte(body), p, "Packages",
		factory, &ar_v3.BulkDeleteArtifactsParams{},
		"art", "ver", "reg", false,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registry not found")
}

// dryRun=true confirmed; actual-run body is valid JSON for the API client's
// generic map but type-mismatches our strict struct, exercising the
// "failed to parse actual bulk delete response" branch.
func TestExecuteWithDryRunResponse_DryRunActualRunBadJSON(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	ts := scriptedServer(t, []scriptedResponse{
		// "success" should be int; passing string makes our struct unmarshal fail
		// while the API client's map[string]interface{} parser still succeeds.
		{200, `{"dryRun": false, "success": "not-a-number"}`},
	})
	factory := newTestFactory(t, ts)

	p := progress.NewConsoleReporter()
	body := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	err := executeWithDryRunResponse(
		[]byte(body), p, "Packages",
		factory, &ar_v3.BulkDeleteArtifactsParams{},
		"art", "ver", "reg", false,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse actual bulk delete response")
}

// dryRun=true confirmed and actual run succeeds with failed-packages list,
// covering the failures-loop in the actual-run branch.
func TestExecuteWithDryRunResponse_DryRunActualRunHasFailures(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"dryRun": false, "success": 1, "failed": 2, "total": 3, "successPackages": ["a@1.0.0"], "failedPackages": ["b@1", "c@1"], "message": "partial"}`},
	})
	factory := newTestFactory(t, ts)

	p := progress.NewConsoleReporter()
	body := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	err := executeWithDryRunResponse(
		[]byte(body), p, "Packages",
		factory, &ar_v3.BulkDeleteArtifactsParams{},
		"art", "ver", "reg", false,
	)
	assert.NoError(t, err)
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

	for _, name := range []string{"registry", "version", "force", "dry-run"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "missing flag: %s", name)
	}
	annotations := cmd.Flags().Lookup("registry").Annotations
	assert.Contains(t, annotations, "cobra_annotation_bash_completion_one_required_flag")
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

// Valid --version exercises the post-validation step ("version expression validated").
func TestNewDeleteArtifactCmd_ValidVersionPatternFullRun(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`},
		{200, `{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`},
	})
	factory := newTestFactory(t, ts)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--version", "1.*"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

// Force=y confirms then aborts at the dry-run prompt (using --dry-run=false
// short-circuits the second prompt entirely - the second response is just
// the actual delete result).
func TestNewDeleteArtifactCmd_ForceConfirmedDryRunDisabled(t *testing.T) {
	withStdin(t, strings.NewReader("y\n"))
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"]}`},
	})
	factory := newTestFactory(t, ts)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--force", "--dry-run=false"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestNewDeleteArtifactCmd_ForceUserCancels(t *testing.T) {
	withStdin(t, strings.NewReader("n\n"))
	cmd := NewDeleteArtifactCmd(&cmdutils.Factory{})
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--force"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by user")
}

// Force prompt with a reader that returns an error -> covers the rErr branch.
func TestNewDeleteArtifactCmd_ForcePromptReadError(t *testing.T) {
	withStdin(t, iotest.ErrReader(io.ErrUnexpectedEOF))
	cmd := NewDeleteArtifactCmd(&cmdutils.Factory{})
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg", "--force"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read confirmation")
}

// Happy path with org/project set, dry-run flow, user aborts at the dry-run prompt.
// Covers: org/project params branches, executeBulkDelete success, executeWithDryRunResponse dry-run prompt cancel.
func TestNewDeleteArtifactCmd_DryRunHappyPathThenAbort(t *testing.T) {
	withGlobalConfig(t, "acct-1", "org-1", "proj-1")
	withStdin(t, strings.NewReader("n\n"))

	dryRunBody := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"], "registry": "myreg", "versionPattern": "*"}`
	ts := scriptedServer(t, []scriptedResponse{{200, dryRunBody}})
	factory := newTestFactory(t, ts)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by user")
}

// Full happy path: dry-run -> user confirms -> real run -> success completion.
// Covers cmd.Execute through to "Bulk delete completed successfully".
func TestNewDeleteArtifactCmd_DryRunConfirmAndComplete(t *testing.T) {
	withGlobalConfig(t, "acct-1", "", "") // exercises the "no org/project" branches
	withStdin(t, strings.NewReader("y\n"))

	dryRunBody := `{"dryRun": true, "success": 1, "successPackages": ["a@1.0.0"]}`
	realRunBody := `{"dryRun": false, "success": 1, "total": 1, "successPackages": ["a@1.0.0"], "message": "deleted"}`
	ts := scriptedServer(t, []scriptedResponse{
		{200, dryRunBody},
		{200, realRunBody},
	})
	factory := newTestFactory(t, ts)

	cmd := NewDeleteArtifactCmd(factory)
	cmd.SetArgs([]string{"valid-pkg", "--registry", "myreg"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
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
