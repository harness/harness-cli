package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/internal/api/ar_v3"
)

const (
	// JWTTokenPrefix marks a CIManager-issued JWT so we route it via the
	// Authorization header rather than the API-key header.
	JWTTokenPrefix = "CIManager"
)

// IsJWTToken reports whether the given auth token should be routed via the
// Authorization header (JWT) instead of x-api-key (PAT / API key).
func IsJWTToken(token string) bool {
	return strings.HasPrefix(token, JWTTokenPrefix)
}

// SetAuthHeader writes the correct auth header for the current AuthToken.
// JWT tokens (CIManager-prefixed) go to Authorization; everything else goes
// to x-api-key. Only one header is ever set — mixing both causes gateways
// that length-check x-api-key to reject the request outright.
func SetAuthHeader(req *http.Request) {
	if IsJWTToken(config.Global.AuthToken) {
		req.Header.Set("Authorization", config.Global.AuthToken)
	} else {
		req.Header.Set("x-api-key", config.Global.AuthToken)
	}
}

// GetXApiKeyOptionAR
// TODO Generics will be difficult coz of RequestEditors but there are possibility of optimisations
func GetXApiKeyOptionAR() func(client *ar.Client) error {
	return func(client *ar.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			SetAuthHeader(req)
			req.Header.Set("User-Agent", config.UserAgent())
			return nil
		})
		return nil
	}
}

func GetAuthOptionARPKG() func(client *ar_pkg.Client) error {
	return func(client *ar_pkg.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			SetAuthHeader(req)
			req.Header.Set("User-Agent", config.UserAgent())
			return nil
		})
		return nil
	}
}

func GetXApiKeyOptionARV2() func(client *ar_v2.Client) error {
	return func(client *ar_v2.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			SetAuthHeader(req)
			req.Header.Set("User-Agent", config.UserAgent())
			return nil
		})
		return nil
	}
}

func GetXApiKeyOptionARV3() func(client *ar_v3.Client) error {
	return func(client *ar_v3.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			SetAuthHeader(req)
			req.Header.Set("User-Agent", config.UserAgent())
			return nil
		})
		return nil
	}
}
