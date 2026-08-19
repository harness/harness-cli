package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/util/common/auth"
)

// Scope is the account/org/project triple. loadTestManager reads it as query
// parameters, never as headers or path segments.
type Scope struct {
	AccountID string
	OrgID     string
	ProjectID string
}

func (s Scope) values() url.Values {
	v := url.Values{}
	if s.AccountID != "" {
		v.Set("accountIdentifier", s.AccountID)
	}
	if s.OrgID != "" {
		v.Set("organizationIdentifier", s.OrgID)
	}
	if s.ProjectID != "" {
		v.Set("projectIdentifier", s.ProjectID)
	}
	return v
}

// request describes a single call. Query holds endpoint-specific parameters;
// the client's scope is merged in afterwards and takes precedence.
type request struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
}

// do performs the request and decodes a JSON response into out. Passing a nil
// out discards the body. A non-2xx status is returned as *APIError.
func (c *Client) do(ctx context.Context, req request, out any) error {
	raw, err := c.doRaw(ctx, req)
	if err != nil {
		return err
	}

	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding response from %s %s: %w", req.Method, req.Path, err)
	}

	return nil
}

// doRaw returns the undecoded response body, for the handful of endpoints that
// answer with YAML or CSV rather than JSON.
func (c *Client) doRaw(ctx context.Context, req request) ([]byte, error) {
	endpoint, err := c.resolveURL(req)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if req.Body != nil {
		encoded, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body for %s %s: %w", req.Method, req.Path, err)
		}
		body = bytes.NewReader(encoded)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, endpoint, body)
	if err != nil {
		return nil, err
	}

	// A JWT goes to Authorization and a PAT to x-api-key, never both: a gateway
	// that length-checks x-api-key rejects a request carrying the pair.
	auth.SetAuthHeader(httpReq)
	httpReq.Header.Set("User-Agent", config.UserAgent())
	httpReq.Header.Set("Accept", "application/json")
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.transportFor(req.Method).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling %s %s: %w", req.Method, req.Path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s %s: %w", req.Method, req.Path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(req.Method, req.Path, resp.StatusCode, payload)
	}

	return payload, nil
}

func (c *Client) transportFor(method string) *http.Client {
	if method == http.MethodGet || method == http.MethodHead {
		return c.read
	}
	return c.write
}

func (c *Client) resolveURL(req request) (string, error) {
	parsed, err := url.Parse(c.baseURL + "/" + strings.TrimLeft(req.Path, "/"))
	if err != nil {
		return "", fmt.Errorf("building URL for %s: %w", req.Path, err)
	}

	query := parsed.Query()
	for key, values := range req.Query {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	for key, values := range c.scope.values() {
		query.Set(key, values[0])
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}
