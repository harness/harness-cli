package gitlab

import (
	"github.com/google/go-containerregistry/pkg/authn"
)

// gitlabKeychain implements authn.Keychain for GitLab authentication
type gitlabKeychain struct {
	username string
	password string
	host     string
}

// NewGitlabKeychain creates a keychain for GitLab authentication
func NewGitlabKeychain(username, password, host string) authn.Keychain {
	return &gitlabKeychain{
		username: username,
		password: password,
		host:     host,
	}
}

// Resolve implements authn.Keychain
func (k *gitlabKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	// Only return credentials for GitLab hosts
	targetHost := target.RegistryStr()
	if targetHost != k.host && targetHost != "registry."+k.host && targetHost != k.host+":443" {
		// Not a GitLab host, return Anonymous so the next keychain in the chain can be tried
		return authn.Anonymous, nil
	}

	// GitLab Container Registry requires OAuth2 authentication
	// The Personal Access Token should be used with "oauth2" as the username
	return authn.FromConfig(authn.AuthConfig{
		Username: "oauth2",    // GitLab Container Registry requires this for PAT
		Password: k.password,  // The Personal Access Token
	}), nil
}
