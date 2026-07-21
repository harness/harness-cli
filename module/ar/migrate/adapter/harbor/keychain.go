package harbor

import (
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
)

type harborKeychain struct {
	username string
	password string
	hostname string
}

func NewHarborKeychain(username, password, hostname string) authn.Keychain {
	return harborKeychain{
		username: username,
		password: password,
		hostname: hostname,
	}
}

func (h harborKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	serverURL, err := url.Parse("https://" + r.String())
	if err != nil {
		return authn.Anonymous, nil
	}

	if h.username == "" || h.password == "" {
		return authn.Anonymous, nil
	}

	if strings.EqualFold(serverURL.Hostname(), h.hostname) {
		return harborAuthenticator{h.username, h.password}, nil
	}
	return authn.Anonymous, nil
}

type harborAuthenticator struct{ username, password string }

func (h harborAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username: h.username,
		Password: h.password,
	}, nil
}
