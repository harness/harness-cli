package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/internal/api/ar_v2"
)

const (
	JWTTokenPrefix = "CIManager"
)

// GetXApiKeyOptionAR
// TODO Generics will be difficult coz of RequestEditors but there are possibility of optimisations
func GetXApiKeyOptionAR() func(client *ar.Client) error {
	return func(client *ar.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("x-api-key", config.Global.AuthToken)
			return nil
		})
		return nil
	}
}

func GetAuthOptionARPKG() func(client *ar_pkg.Client) error {
	return func(client *ar_pkg.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			if strings.HasPrefix(config.Global.AuthToken, JWTTokenPrefix) {
				// JWT token - use Authorization header
				req.Header.Set("Authorization", config.Global.AuthToken)
			} else {
				// API key - use x-api-key header
				req.Header.Set("x-api-key", config.Global.AuthToken)
			}
			return nil
		})
		return nil
	}
}

func GetXApiKeyOptionARV2() func(client *ar_v2.Client) error {
	return func(client *ar_v2.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("x-api-key", config.Global.AuthToken)
			return nil
		})
		return nil
	}
}
