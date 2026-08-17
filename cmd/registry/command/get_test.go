package command

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withRegistryGlobals temporarily sets config.Global fields and restores them.
func withRegistryGlobals(t *testing.T, account, org, project, format string) {
	t.Helper()
	orig := config.Global
	config.Global.AccountID = account
	config.Global.OrgID = org
	config.Global.ProjectID = project
	config.Global.Format = format
	t.Cleanup(func() { config.Global = orig })
}

// newARTestFactory wires a Factory whose RegistryHttpClient hits the given server URL.
func newARTestFactory(t *testing.T, serverURL string) *cmdutils.Factory {
	t.Helper()
	client, err := ar.NewClientWithResponses(serverURL)
	require.NoError(t, err)
	return &cmdutils.Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses { return client },
	}
}

// captureStdout swaps os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	runErr := fn()

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	os.Stdout = orig
	return buf.String(), runErr
}

func executeGetCmd(f *cmdutils.Factory, args ...string) error {
	cmd := NewGetRegistryCmd(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// Existing registry: the single-GET path is used and the registry is printed.
func TestGetRegistryCmd_SingleGetSuccess(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")

	var gotPath, gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"SUCCESS","data":{"identifier":"myreg","packageType":"DOCKER","description":"d","url":"https://example.com/myreg"}}`))
	}))
	t.Cleanup(ts.Close)

	out, err := captureStdout(t, func() error {
		return executeGetCmd(newARTestFactory(t, ts.URL), "myreg")
	})
	assert.NoError(t, err)
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/registry/acct/myreg/+", gotPath)
	assert.Contains(t, out, "myreg")
}

// Missing registry: explicit non-zero error instead of an empty JSON list.
func TestGetRegistryCmd_SingleGetNotFound(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"registry not found"}`))
	}))
	t.Cleanup(ts.Close)

	err := executeGetCmd(newARTestFactory(t, ts.URL), "myreg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "myreg")
	assert.Contains(t, err.Error(), "not found")
}

// Non-200 without a structured body still surfaces a explicit error.
func TestGetRegistryCmd_SingleGetServerError(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	t.Cleanup(ts.Close)

	err := executeGetCmd(newARTestFactory(t, ts.URL), "myreg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get registry 'myreg'")
}

// Single get also works in the default table format.
func TestGetRegistryCmd_SingleGetTableFormat(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"SUCCESS","data":{"identifier":"myreg","packageType":"DOCKER","url":"u"}}`))
	}))
	t.Cleanup(ts.Close)

	assert.NoError(t, executeGetCmd(newARTestFactory(t, ts.URL), "myreg"))
}

// No positional arg keeps the previous search/list behavior.
func TestGetRegistryCmd_ListWithoutNameStillWorks(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")

	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"SUCCESS","data":{"registries":[{"identifier":"reg1","packageType":"DOCKER","type":"VIRTUAL","url":"u"}],"itemCount":1,"pageCount":1,"pageIndex":0}}`))
	}))
	t.Cleanup(ts.Close)

	out, err := captureStdout(t, func() error {
		return executeGetCmd(newARTestFactory(t, ts.URL))
	})
	assert.NoError(t, err)
	assert.False(t, strings.HasSuffix(gotPath, "/+"), "list path must not use the single-GET endpoint")
	assert.Contains(t, out, "reg1")
}

// More than one positional arg is rejected.
func TestGetRegistryCmd_TooManyArgs(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")
	err := executeGetCmd(newARTestFactory(t, "http://127.0.0.1:1"), "a", "b")
	assert.Error(t, err)
}
