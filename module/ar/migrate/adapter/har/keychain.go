package har

import (
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
)

type harKeychain struct {
	username string
	password string
	hostname string
}

func NewHarKeychain(username, password, hostname string) authn.Keychain {
	return harKeychain{
		username: username,
		password: password,
		hostname: hostname,
	}
}

func (g harKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	serverURL, err := url.Parse("https://" + r.String())
	if err != nil {
		return authn.Anonymous, nil
	}

	if g.password == "" {
		return authn.Anonymous, nil
	}

	if strings.EqualFold(serverURL.Hostname(), g.hostname) {
		username := g.username
		if username == "" {
			username = "x-token"
		}
		return harAuthenticator{username, g.password}, nil
	}
	return authn.Anonymous, nil
}

type harAuthenticator struct{ username, password string }

func (g harAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username: g.username,
		Password: g.password,
	}, nil
}
