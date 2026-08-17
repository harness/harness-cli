package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeCreateCmd(f *cmdutils.Factory, args ...string) error {
	cmd := NewCreateRegistryCmd(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// Successful create: POST /registry with the identifier and a VIRTUAL config body.
func TestCreateRegistryCmd_Success(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")

	var gotMethod, gotPath, gotSpaceRef, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotSpaceRef = r.URL.Query().Get("space_ref")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"SUCCESS","data":{"identifier":"newreg","packageType":"DOCKER","url":"https://example.com/newreg"}}`))
	}))
	t.Cleanup(ts.Close)

	out, err := captureStdout(t, func() error {
		return executeCreateCmd(newARTestFactory(t, ts.URL), "newreg", "--description", "d")
	})
	assert.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/registry", gotPath)
	assert.Equal(t, "acct", gotSpaceRef)
	assert.Contains(t, gotBody, `"identifier":"newreg"`)
	assert.Contains(t, gotBody, `"packageType":"DOCKER"`)
	assert.Contains(t, gotBody, `"type":"VIRTUAL"`)
	assert.Contains(t, gotBody, `"description":"d"`)
	assert.Contains(t, out, "newreg")
}

// Server-side failure surfaces the server's error message.
func TestCreateRegistryCmd_ServerError(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"BAD_REQUEST","message":"identifier already exists"}`))
	}))
	t.Cleanup(ts.Close)

	err := executeCreateCmd(newARTestFactory(t, ts.URL), "newreg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identifier already exists")
}

// The identifier positional arg is required.
func TestCreateRegistryCmd_RequiresIdentifier(t *testing.T) {
	withRegistryGlobals(t, "acct", "", "", "json")
	err := executeCreateCmd(newARTestFactory(t, "http://127.0.0.1:1"))
	assert.Error(t, err)
}
