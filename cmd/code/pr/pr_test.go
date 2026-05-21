package pr

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/util/client/code"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			_, _ = w.Write([]byte(`{"error":"unexpected extra request"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(responses[i].status)
		_, _ = w.Write([]byte(responses[i].body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestFactory(t *testing.T, ts *httptest.Server) *cmdutils.Factory {
	t.Helper()
	return &cmdutils.Factory{
		CodeClient: func() *code.Client {
			c := code.NewClientWithBaseURL(ts.URL, "test-token", "test-account")
			c.SetRetryCount(0)
			return c
		},
	}
}

func newUnreachableFactory() *cmdutils.Factory {
	return &cmdutils.Factory{
		CodeClient: func() *code.Client {
			c := code.NewClientWithBaseURL("http://127.0.0.1:1", "tok", "acct")
			c.SetRetryCount(0)
			return c
		},
	}
}

// ---------------------------------------------------------------------
// pr list
// ---------------------------------------------------------------------

func TestList_HappyPath(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `[{"number":1,"title":"feat: add foo","state":"open","author":{"display_name":"Alice"},"source_branch":"feat/foo","target_branch":"main"},{"number":2,"title":"fix: bar","state":"merged","author":{"display_name":"Bob"},"source_branch":"fix/bar","target_branch":"main"}]`},
	})
	f := newTestFactory(t, ts)

	cmd := newListCmd(f)
	cmd.SetArgs([]string{"--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestList_EmptyResult(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `[]`},
	})
	f := newTestFactory(t, ts)

	cmd := newListCmd(f)
	cmd.SetArgs([]string{"--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestList_WithStateFilter(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `[{"number":1,"title":"feat: add foo","state":"open","author":{"display_name":"Alice"},"source_branch":"feat/foo","target_branch":"main"}]`},
	})
	f := newTestFactory(t, ts)

	cmd := newListCmd(f)
	cmd.SetArgs([]string{"--repo", "acct/org/proj/repo", "--state", "open"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestList_JSONOutput(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `[{"number":1,"title":"feat: add foo","state":"open","author":{"display_name":"Alice"},"source_branch":"feat/foo","target_branch":"main"}]`},
	})
	f := newTestFactory(t, ts)

	cmd := newListCmd(f)
	cmd.SetArgs([]string{"--repo", "acct/org/proj/repo", "--json", "number,title"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestList_401Unauthorized(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{401, `{"message":"unauthorized"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newListCmd(f)
	cmd.SetArgs([]string{"--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestList_TransportError(t *testing.T) {
	f := newUnreachableFactory()

	cmd := newListCmd(f)
	cmd.SetArgs([]string{"--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.Error(t, cmd.Execute())
}

func TestList_AutoDetectsRepo(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `[]`},
	})
	f := newTestFactory(t, ts)

	cmd := newListCmd(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	// When run inside a harness git repo, auto-detection succeeds
	assert.NoError(t, cmd.Execute())
}

// ---------------------------------------------------------------------
// pr view
// ---------------------------------------------------------------------

func TestView_HappyPath(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":42,"title":"feat: something","state":"open","author":{"display_name":"Alice"},"source_branch":"feat/x","target_branch":"main","description":"A good PR","stats":{"additions":10,"deletions":3,"files_changed":2,"commits":1},"created":1716249600000}`},
	})
	f := newTestFactory(t, ts)

	cmd := newViewCmd(f)
	cmd.SetArgs([]string{"42", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestView_InvalidNumber(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newViewCmd(f)
	cmd.SetArgs([]string{"abc", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PR number")
}

func TestView_404NotFound(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{404, `{"message":"pull request not found"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newViewCmd(f)
	cmd.SetArgs([]string{"999", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestView_JSONOutput(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":42,"title":"feat: something","state":"open","author":{"display_name":"Alice"},"source_branch":"feat/x","target_branch":"main","stats":{"additions":10,"deletions":3,"files_changed":2,"commits":1}}`},
	})
	f := newTestFactory(t, ts)

	cmd := newViewCmd(f)
	cmd.SetArgs([]string{"42", "--repo", "acct/org/proj/repo", "--json", "number,title,state"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestView_NoArgs(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newViewCmd(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------
// pr create
// ---------------------------------------------------------------------

func TestCreate_HappyPath(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{201, `{"number":5,"title":"feat: new thing","state":"open","source_branch":"feat/new","target_branch":"main","pr_url":"https://example.com/pr/5"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "feat: new thing", "--head", "feat/new", "--base", "main", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestCreate_MissingTitle(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--head", "feat/new", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--title is required")
}

func TestCreate_MissingHead(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "something", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--head is required")
}

func TestCreate_DryRun(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "feat: test", "--head", "feat/test", "--base", "main", "--dry-run", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestCreate_DryRunWithDraft(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "wip: test", "--head", "feat/test", "--draft", "--dry-run", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestCreate_WithBody(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{201, `{"number":6,"title":"feat: body test","state":"open","source_branch":"feat/body","target_branch":"main"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "feat: body test", "--head", "feat/body", "--body", "This is the description", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestCreate_JSONOutput(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{201, `{"number":7,"title":"feat: json","state":"open","source_branch":"feat/json","target_branch":"main","pr_url":"https://example.com/pr/7"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "feat: json", "--head", "feat/json", "--json", "number,pr_url", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestCreate_APIError(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{400, `{"message":"source branch does not exist"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "feat: bad", "--head", "nonexistent", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestCreate_BodyFileNotFound(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCreateCmd(f)
	cmd.SetArgs([]string{"--title", "feat: file", "--head", "feat/file", "--body-file", "/nonexistent/path.md", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not open body file")
}

// ---------------------------------------------------------------------
// pr merge
// ---------------------------------------------------------------------

func TestMerge_HappyPath(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":3,"title":"feat: merge me","state":"open","source_sha":"abc123"}`},
		{200, `{"sha":"def456","mergeable":true,"conflict_files":[],"rule_violations":[]}`},
	})
	f := newTestFactory(t, ts)

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"3", "--yes", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestMerge_AlreadyMerged(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":3,"title":"feat: done","state":"merged","source_sha":"abc123"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"3", "--yes", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestMerge_RequiresConfirmation(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":3,"title":"feat: needs confirm","state":"open","source_sha":"abc123"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"3", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "destructive operation")
}

func TestMerge_DryRun(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":3,"title":"feat: dry","state":"open","source_sha":"abc123"}`},
		{200, `{"sha":"","mergeable":true,"conflict_files":[],"rule_violations":[]}`},
	})
	f := newTestFactory(t, ts)

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"3", "--dry-run", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestMerge_InvalidNumber(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"xyz", "--yes", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PR number")
}

func TestMerge_404NotFound(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{404, `{"message":"pull request not found"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"999", "--yes", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestMerge_JSONOutput(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"number":3,"title":"feat: merged","state":"merged","source_sha":"abc123"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{"3", "--yes", "--json", "number,state", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestMerge_NoArgs(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newMergeCmd(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------
// pr comment
// ---------------------------------------------------------------------

func TestComment_HappyPath(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{201, `{"id":101,"text":"LGTM","author":{"display_name":"Alice"},"created":1716249600}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"5", "--body", "LGTM", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestComment_MissingBody(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"5", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--body or --body-file is required")
}

func TestComment_DryRun(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"5", "--body", "test comment", "--dry-run", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestComment_InvalidNumber(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"abc", "--body", "test", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PR number")
}

func TestComment_401Unauthorized(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{401, `{"message":"unauthorized"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"5", "--body", "test", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestComment_JSONOutput(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{201, `{"id":102,"text":"Approved","author":{"display_name":"Bob"},"created":1716249600}`},
	})
	f := newTestFactory(t, ts)

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"5", "--body", "Approved", "--json", "id,text", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestComment_BodyFileNotFound(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{"5", "--body-file", "/nonexistent/file.txt", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not open body file")
}

func TestComment_NoArgs(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newCommentCmd(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------
// pr checks
// ---------------------------------------------------------------------

func TestChecks_AllPassed(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"commit_sha":"abc123","checks":[{"required":true,"bypassable":false,"check":{"identifier":"ci/build","status":"success","link":"https://example.com/build/1"}},{"required":false,"bypassable":true,"check":{"identifier":"ci/lint","status":"success","link":"https://example.com/lint/1"}}]}`},
	})
	f := newTestFactory(t, ts)

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{"10", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestChecks_NoChecks(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"commit_sha":"abc123","checks":[]}`},
	})
	f := newTestFactory(t, ts)

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{"10", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestChecks_JSONOutput(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{200, `{"commit_sha":"abc123","checks":[{"required":true,"bypassable":false,"check":{"identifier":"ci/build","status":"success","link":"https://example.com/build/1"}}]}`},
	})
	f := newTestFactory(t, ts)

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{"10", "--repo", "acct/org/proj/repo", "--json", "check.identifier,check.status"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.NoError(t, cmd.Execute())
}

func TestChecks_InvalidNumber(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{"abc", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PR number")
}

func TestChecks_404NotFound(t *testing.T) {
	ts := scriptedServer(t, []scriptedResponse{
		{404, `{"message":"pull request not found"}`},
	})
	f := newTestFactory(t, ts)

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{"999", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestChecks_TransportError(t *testing.T) {
	f := newUnreachableFactory()

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{"10", "--repo", "acct/org/proj/repo"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	assert.Error(t, cmd.Execute())
}

func TestChecks_NoArgs(t *testing.T) {
	f := &cmdutils.Factory{}

	cmd := newChecksCmd(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------
// Helpers: resolveRepoRef, printJSON, filterFields
// ---------------------------------------------------------------------

func TestResolveRepoRef_Explicit(t *testing.T) {
	ref, err := resolveRepoRef("acct/org/proj/repo")
	require.NoError(t, err)
	assert.Equal(t, "acct/org/proj/repo", ref)
}

func TestResolveRepoRef_AutoDetect(t *testing.T) {
	// When run inside a Harness Code git repo, auto-detection succeeds
	ref, err := resolveRepoRef("")
	require.NoError(t, err)
	assert.NotEmpty(t, ref)
}

func TestPrintJSON_NilSliceOutputsEmptyArray(t *testing.T) {
	var nilChecks []code.Check
	err := printJSON(nilChecks, "")
	assert.NoError(t, err)
}

func TestPrintJSON_NilSliceWithFieldsOutputsEmptyArray(t *testing.T) {
	var nilChecks []code.Check
	err := printJSON(nilChecks, "check.identifier")
	assert.NoError(t, err)
}

func TestFilterFields_NestedField(t *testing.T) {
	obj := map[string]interface{}{
		"author": map[string]interface{}{
			"display_name": "Alice",
			"email":        "alice@example.com",
		},
		"number": float64(1),
	}
	result := filterFields(obj, []string{"author.display_name", "number"})
	assert.Equal(t, "Alice", result["author.display_name"])
	assert.Equal(t, float64(1), result["number"])
}

func TestFilterFields_MissingField(t *testing.T) {
	obj := map[string]interface{}{
		"number": float64(1),
	}
	result := filterFields(obj, []string{"title", "number"})
	assert.Equal(t, float64(1), result["number"])
	_, exists := result["title"]
	assert.False(t, exists)
}
