package har

import (
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/rs/zerolog/log"
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
	resourceStr := r.String()
	serverURL, err := url.Parse("https://" + resourceStr)
	if err != nil {
		// Log the error for debugging
		log.Error().
			Str("resource", resourceStr).
			Err(err).
			Msg("HAR keychain: failed to parse resource URL, returning Anonymous")
		return authn.Anonymous, nil
	}

	if g.password == "" {
		log.Warn().Msg("HAR keychain: no password set, returning Anonymous")
		return authn.Anonymous, nil
	}

	hostname := serverURL.Hostname()
	if strings.EqualFold(hostname, g.hostname) {
		username := g.username
		if username == "" {
			username = "x-token"
		}
		log.Info().
			Str("hostname", hostname).
			Str("username", username).
			Msg("HAR keychain: matched hostname, returning credentials")
		return harAuthenticator{username, g.password}, nil
	}
	log.Warn().
		Str("serverHostname", hostname).
		Str("expectedHostname", g.hostname).
		Msg("HAR keychain: hostname mismatch, returning Anonymous")
	return authn.Anonymous, nil
}

type harAuthenticator struct{ username, password string }

func (g harAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username: g.username,
		Password: g.password,
	}, nil
}
