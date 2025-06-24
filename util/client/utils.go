package client

import (
	"fmt"
	"net/url"
	"strings"

	"harness/config"
)

func GetScopeRef() string {
	return GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID)
}

func GetRef(params ...string) string {
	ref := ""
	for _, param := range params {
		if param != "" {
			ref += param + "/"
		}
	}
	if len(ref) > 0 {
		ref = ref[:len(ref)-1]
	}
	return ref
}

func GetSchemeAndHostFromEndpoint(endpoint string) (string, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint: %w", err)
	}

	// If the scheme or host is missing, assume HTTP and retry
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		parsedURL, err = url.Parse("http://" + endpoint)
		if err != nil {
			return "", fmt.Errorf("invalid endpoint with assumed scheme: %w", err)
		}
	}

	// Some inputs like "localhost:8080" get parsed as Path; handle manually
	if parsedURL.Host == "" && parsedURL.Path != "" && !strings.Contains(parsedURL.Path, "/") {
		return parsedURL.Scheme + "://" + parsedURL.Path, nil
	}

	return parsedURL.Scheme + "://" + parsedURL.Host, nil
}
