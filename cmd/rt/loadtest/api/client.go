package api

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

// APIPath routes loadTestManager under its own gateway service name; putting
// it under chaos as /gateway/chaos/lserver/... answers 404.
const APIPath = "/gateway/loadTest/manager/api/v1"

// APIPathEnv overrides APIPath, for an installation publishing loadTestManager
// on its own ingress rather than behind the gateway.
const APIPathEnv = "HARNESS_LOAD_API_PATH"

// apiPath returns the route prefix, honouring APIPathEnv.
func apiPath() string {
	if override := strings.TrimSpace(os.Getenv(APIPathEnv)); override != "" {
		return "/" + strings.Trim(override, "/")
	}
	return APIPath
}

// Client talks to loadTestManager.
type Client struct {
	baseURL string
	scope   Scope

	// Reads are retried, writes go once. Nothing here is a safe write to
	// repeat: a second POST starts a second run, a second PUT cuts a revision.
	read  *http.Client
	write *http.Client

	// parentIdentities maps a load test's unique id to its identity. Watching a
	// run needs it every few seconds and it cannot change under a live client.
	parentMu         sync.Mutex
	parentIdentities map[string]string
}

// Config describes a Client. Server is the Harness host, for example
// https://app.harness.io. No credential is passed: util/common/auth applies
// one per request from the global configuration, as the registry clients do.
type Config struct {
	Server  string
	Scope   Scope
	Timeout time.Duration
}

const defaultTimeout = 60 * time.Second

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	read := newRetryClient()
	read.Timeout = timeout

	return &Client{
		baseURL:          strings.TrimRight(cfg.Server, "/") + apiPath(),
		scope:            cfg.Scope,
		read:             read,
		write:            &http.Client{Timeout: timeout},
		parentIdentities: map[string]string{},
	}
}

// newRetryClient backs off faster than the library default, which spends 7
// seconds before giving up. Someone is waiting at a terminal.
func newRetryClient() *http.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.RetryWaitMin = 500 * time.Millisecond
	client.RetryWaitMax = 2 * time.Second
	client.Logger = nil

	return client.StandardClient()
}
