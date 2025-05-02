package jfrog

import (
	"harness/module/ar/migrate/http"
	"harness/module/ar/migrate/http/auth/basic"
	"harness/module/ar/migrate/types"
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
	}
}

type client struct {
	client   *http.Client
	url      string
	insecure bool
	username string
	password string
}

func (c *client) getFiles(registry string) (interface{}, error) {
	
	return nil, nil
}
