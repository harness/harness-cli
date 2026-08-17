package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withListGlobals temporarily sets config.Global fields and restores them.
func withListGlobals(t *testing.T, account string) {
	t.Helper()
	orig := config.Global
	config.Global.AccountID = account
	config.Global.OrgID = ""
	config.Global.ProjectID = ""
	t.Cleanup(func() { config.Global = orig })
}

// listQueryServer returns an httptest server that records the reg_identifier
// query param and responds with an empty artifact page.
func listQueryServer(t *testing.T, gotRegIdentifier *string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotRegIdentifier = r.URL.Query().Get("reg_identifier")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"SUCCESS","data":{"artifacts":[],"itemCount":0,"pageCount":0,"pageIndex":0}}`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func newListARFactory(t *testing.T, serverURL string) *cmdutils.Factory {
	t.Helper()
	client, err := ar.NewClientWithResponses(serverURL)
	require.NoError(t, err)
	return &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}
}

func executeListCmd(f *cmdutils.Factory, args ...string) error {
	cmd := NewListArtifactCmd(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// A positional registry arg scopes the request to that registry (previously it
// was silently ignored and the listing ran account-wide).
func TestListArtifactCmd_PositionalRegistryScopesRequest(t *testing.T) {
	withListGlobals(t, "acct")
	var gotReg string
	ts := listQueryServer(t, &gotReg)

	err := executeListCmd(newListARFactory(t, ts.URL), "myreg")
	assert.NoError(t, err)
	assert.Equal(t, "myreg", gotReg)
}

// No positional arg and no flag: explicitly account-wide.
func TestListArtifactCmd_NoRegistryRunsAccountWide(t *testing.T) {
	withListGlobals(t, "acct")
	var gotReg string
	ts := listQueryServer(t, &gotReg)

	err := executeListCmd(newListARFactory(t, ts.URL))
	assert.NoError(t, err)
	assert.Equal(t, "", gotReg)
}

// The --registry flag still scopes the request.
func TestListArtifactCmd_RegistryFlagScopesRequest(t *testing.T) {
	withListGlobals(t, "acct")
	var gotReg string
	ts := listQueryServer(t, &gotReg)

	err := executeListCmd(newListARFactory(t, ts.URL), "--registry", "flagreg")
	assert.NoError(t, err)
	assert.Equal(t, "flagreg", gotReg)
}

// Positional arg and --registry agreeing is accepted.
func TestListArtifactCmd_PositionalAndFlagAgree(t *testing.T) {
	withListGlobals(t, "acct")
	var gotReg string
	ts := listQueryServer(t, &gotReg)

	err := executeListCmd(newListARFactory(t, ts.URL), "myreg", "--registry", "myreg")
	assert.NoError(t, err)
	assert.Equal(t, "myreg", gotReg)
}

// Positional arg conflicting with --registry is an error, never a silent widen.
func TestListArtifactCmd_ConflictingRegistryArgs(t *testing.T) {
	withListGlobals(t, "acct")
	var gotReg string
	ts := listQueryServer(t, &gotReg)

	err := executeListCmd(newListARFactory(t, ts.URL), "myreg", "--registry", "other")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting registry arguments")
	assert.Equal(t, "", gotReg, "no request should be issued on conflicting args")
}

// More than one positional arg is rejected.
func TestListArtifactCmd_TooManyArgs(t *testing.T) {
	withListGlobals(t, "acct")
	err := executeListCmd(newListARFactory(t, "http://127.0.0.1:1"), "a", "b")
	assert.Error(t, err)
}
