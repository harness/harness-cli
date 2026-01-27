package mock_jfrog

import (
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
)

type jfrogKeychain struct {
	username string
	password string
	hostname string
}

func NewJfrogKeychain(username, password, hostname string) authn.Keychain {
	return jfrogKeychain{
		username: username,
		password: password,
		hostname: hostname,
	}
}

func (g jfrogKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	serverURL, err := url.Parse("https://" + r.String())
	if err != nil {
		return authn.Anonymous, nil
	}

	if g.username == "" || g.password == "" {
		return authn.Anonymous, nil
	}

	if strings.EqualFold(serverURL.Hostname(), g.hostname) {
		return jfrogAuthenticator{g.username, g.password}, nil
	}
	return authn.Anonymous, nil
}

type jfrogAuthenticator struct{ username, password string }

func (g jfrogAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username: g.username,
		Password: g.password,
	}, nil
}
