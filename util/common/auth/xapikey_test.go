package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/internal/api/ar_v3"

	"github.com/stretchr/testify/assert"
)

func TestIsJWTToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"empty", "", false},
		{"pat", "pat.acct.aaa.bbb", false},
		{"api-key-style", "sat.acct.xxx.yyy", false},
		{"jwt-with-prefix-and-space", "CIManager eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig", true},
		{"jwt-with-prefix-no-space", "CIManagerxxx", true}, // prefix match only; caller controls formatting
		{"raw-jwt-without-prefix", "eyJhbGciOiJIUzI1NiJ9.payload.sig", false},
		{"prefix-lowercase", "cimanager eyJ...", false},
		{"prefix-only", "CIManager", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsJWTToken(tt.token))
		})
	}
}

// withAuthToken swaps config.Global.AuthToken for the duration of a subtest
// and restores it afterwards so tests don't leak state.
func withAuthToken(t *testing.T, token string) {
	t.Helper()
	orig := config.Global.AuthToken
	config.Global.AuthToken = token
	t.Cleanup(func() { config.Global.AuthToken = orig })
}

func TestSetAuthHeader_JWTGoesToAuthorization(t *testing.T) {
	withAuthToken(t, "CIManager eyJhbGciOiJIUzI1NiJ9.payload.sig")

	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/x", nil)
	assert.NoError(t, err)

	SetAuthHeader(req)

	assert.Equal(t, "CIManager eyJhbGciOiJIUzI1NiJ9.payload.sig", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-api-key"),
		"x-api-key must not be set when Authorization is used — gateways length-check x-api-key and would reject a JWT there")
}

func TestSetAuthHeader_PATGoesToXApiKey(t *testing.T) {
	withAuthToken(t, "pat.acct.aaa.bbb")

	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/x", nil)
	assert.NoError(t, err)

	SetAuthHeader(req)

	assert.Equal(t, "pat.acct.aaa.bbb", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestSetAuthHeader_EmptyTokenStillUsesXApiKey(t *testing.T) {
	// Empty tokens shouldn't crash; they route to x-api-key by default (matches
	// pre-existing behavior — the caller decides whether to send a request at all).
	withAuthToken(t, "")

	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/x", nil)
	assert.NoError(t, err)

	SetAuthHeader(req)

	assert.Equal(t, "", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestGetXApiKeyOptionAR_JWT(t *testing.T) {
	withAuthToken(t, "CIManager eyJ.payload.sig")
	c := &ar.Client{}
	assert.NoError(t, GetXApiKeyOptionAR()(c))
	req, _ := http.NewRequest(http.MethodGet, "https://x/y", nil)
	assert.NoError(t, c.RequestEditors[0](context.Background(), req))
	assert.Equal(t, "CIManager eyJ.payload.sig", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-api-key"))
	assert.NotEmpty(t, req.Header.Get("User-Agent"))
}

func TestGetXApiKeyOptionAR_PAT(t *testing.T) {
	withAuthToken(t, "pat.a.b.c")
	c := &ar.Client{}
	assert.NoError(t, GetXApiKeyOptionAR()(c))
	req, _ := http.NewRequest(http.MethodGet, "https://x/y", nil)
	assert.NoError(t, c.RequestEditors[0](context.Background(), req))
	assert.Equal(t, "pat.a.b.c", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestGetAuthOptionARPKG_RoutesByTokenShape(t *testing.T) {
	// JWT path
	withAuthToken(t, "CIManager eyJ.payload.sig")
	c := &ar_pkg.Client{}
	assert.NoError(t, GetAuthOptionARPKG()(c))
	req, _ := http.NewRequest(http.MethodGet, "https://x/y", nil)
	assert.NoError(t, c.RequestEditors[0](context.Background(), req))
	assert.Equal(t, "CIManager eyJ.payload.sig", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-api-key"))

	// PAT path
	withAuthToken(t, "pat.a.b.c")
	c2 := &ar_pkg.Client{}
	assert.NoError(t, GetAuthOptionARPKG()(c2))
	req2, _ := http.NewRequest(http.MethodGet, "https://x/y", nil)
	assert.NoError(t, c2.RequestEditors[0](context.Background(), req2))
	assert.Equal(t, "pat.a.b.c", req2.Header.Get("x-api-key"))
	assert.Empty(t, req2.Header.Get("Authorization"))
}

func TestGetXApiKeyOptionARV2_RoutesByTokenShape(t *testing.T) {
	withAuthToken(t, "CIManager eyJ.p.s")
	c := &ar_v2.Client{}
	assert.NoError(t, GetXApiKeyOptionARV2()(c))
	req, _ := http.NewRequest(http.MethodGet, "https://x/y", nil)
	assert.NoError(t, c.RequestEditors[0](context.Background(), req))
	assert.Equal(t, "CIManager eyJ.p.s", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-api-key"))
}

func TestGetXApiKeyOptionARV3_RoutesByTokenShape(t *testing.T) {
	withAuthToken(t, "pat.acct.aa.bb")
	c := &ar_v3.Client{}
	assert.NoError(t, GetXApiKeyOptionARV3()(c))
	req, _ := http.NewRequest(http.MethodGet, "https://x/y", nil)
	assert.NoError(t, c.RequestEditors[0](context.Background(), req))
	assert.Equal(t, "pat.acct.aa.bb", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestSetAuthHeader_UsesSetNotAdd(t *testing.T) {
	// If a caller runs SetAuthHeader twice on the same request (e.g. a retry
	// path that re-decorates), we must not accumulate duplicate values.
	withAuthToken(t, "CIManager eyJhbGciOiJIUzI1NiJ9.payload.sig")

	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid/x", nil)
	SetAuthHeader(req)
	SetAuthHeader(req)

	assert.Equal(t, []string{"CIManager eyJhbGciOiJIUzI1NiJ9.payload.sig"}, req.Header.Values("Authorization"),
		"must use Set (single value), not Add (append), even after repeated invocations")
}
