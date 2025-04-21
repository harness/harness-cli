package jfrog

import (
	"harness/clients/ar"
	"harness/config"
	"harness/module/ar/http"
	"harness/module/ar/http/auth/basic"
	"harness/module/ar/types"
	http2 "net/http"
)

// newClient constructs a jfrog client
func newClient(reg *types.RegistryConfig) *client {
	username, password := "", ""

	username = reg.Credentials.Username
	password = reg.Credentials.Token

	return &client{
		client: http.NewClient(
			&http2.Client{
				Transport: http.GetHTTPTransport(http.WithInsecure(true)),
			},
			basic.NewAuthorizer(username, password),
		),
		url:      reg.Endpoint,
		insecure: true,
		username: username,
		password: password,
		apiClient: ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
			config.Global.OrgID, config.Global.ProjectID),
	}
}

type client struct {
	apiClient *ar.Client
	client    *http.Client
	url       string
	insecure  bool
	username  string
	password  string
}
