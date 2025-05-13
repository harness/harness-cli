package auth

import (
	"context"
	"harness/internal/api/ar"
	"harness/internal/api/ar_pkg"
	"net/http"

	"harness/config"
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

func GetXApiKeyOptionARPKG() func(client *ar_pkg.Client) error {
	return func(client *ar_pkg.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("x-api-key", config.Global.AuthToken)
			return nil
		})
		return nil
	}
}
