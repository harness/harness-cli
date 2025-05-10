package auth

import (
	"context"
	"harness/config"
	"harness/internal/api/ar"
	"net/http"
)

func GetXApiKeyOption() func(client *ar.Client) error {
	return func(client *ar.Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("x-api-key", config.Global.AuthToken)
			return nil
		})
		return nil
	}
}
