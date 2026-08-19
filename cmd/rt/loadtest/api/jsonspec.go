package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// JSONSpec declares endpoints the server compiles into a script, so a test can
// be defined without code. Locust only, and snake_case because it is a file.
type JSONSpec struct {
	Config    JSONSpecConfig     `json:"config"`
	Endpoints []JSONSpecEndpoint `json:"endpoints"`
}

// JSONSpecConfig is the shared setting for every endpoint in the spec.
type JSONSpecConfig struct {
	// Host is the base URL each endpoint path is appended to.
	Host string `json:"host"`
	// MinWait and MaxWait bound the pause between requests, in seconds. Unset
	// means users request as fast as they can.
	MinWait *int `json:"min_wait,omitempty"`
	MaxWait *int `json:"max_wait,omitempty"`
}

// JSONSpecEndpoint is one request in the spec.
type JSONSpecEndpoint struct {
	Name        string            `json:"name"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        map[string]any    `json:"body,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Assertions  *JSONSpecAsserts  `json:"assertions,omitempty"`
	// Extract pulls a response value into a variable later endpoints can
	// reference, carrying a login token through a multi-step flow.
	Extract *JSONSpecExtract `json:"extract,omitempty"`
}

// JSONSpecAsserts marks a response as a failure when it does not hold.
type JSONSpecAsserts struct {
	StatusCode        *int `json:"status_code,omitempty"`
	MaxResponseTimeMs *int `json:"max_response_time_ms,omitempty"`
}

type JSONSpecExtract struct {
	VariableName string `json:"variable_name"`
	JSONPath     string `json:"json_path"`
}

// JSONSpecMethods are the HTTP methods an endpoint may use.
var JSONSpecMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// Validate reports the problems the API would reject, so a malformed spec
// fails locally with a message naming the offending endpoint.
func (s JSONSpec) Validate() error {
	if strings.TrimSpace(s.Config.Host) == "" {
		return fmt.Errorf("config.host is required: give the base URL the endpoint paths are appended to")
	}
	if len(s.Endpoints) == 0 {
		return fmt.Errorf("endpoints is required: declare at least one request to send")
	}
	if s.Config.MinWait != nil && s.Config.MaxWait != nil && *s.Config.MinWait > *s.Config.MaxWait {
		return fmt.Errorf("config.min_wait (%d) cannot be greater than config.max_wait (%d)", *s.Config.MinWait, *s.Config.MaxWait)
	}

	seen := map[string]bool{}
	for i, endpoint := range s.Endpoints {
		// Endpoints are reported by index as well as name, since a spec with a
		// missing name has nothing else to identify it by.
		where := fmt.Sprintf("endpoints[%d]", i)
		if endpoint.Name != "" {
			where = fmt.Sprintf("%s (%q)", where, endpoint.Name)
		}

		if strings.TrimSpace(endpoint.Name) == "" {
			return fmt.Errorf("%s: name is required; it labels the endpoint in run results", where)
		}
		if seen[endpoint.Name] {
			return fmt.Errorf("%s: duplicate name; results are grouped by name, so each must be unique", where)
		}
		seen[endpoint.Name] = true

		if strings.TrimSpace(endpoint.Path) == "" {
			return fmt.Errorf("%s: path is required", where)
		}
		method := strings.ToUpper(strings.TrimSpace(endpoint.Method))
		if method == "" {
			return fmt.Errorf("%s: method is required, one of %s", where, strings.Join(JSONSpecMethods, ", "))
		}
		if !containsString(JSONSpecMethods, method) {
			return fmt.Errorf("%s: unsupported method %q, expected one of %s", where, endpoint.Method, strings.Join(JSONSpecMethods, ", "))
		}
		if endpoint.Extract != nil {
			if endpoint.Extract.VariableName == "" || endpoint.Extract.JSONPath == "" {
				return fmt.Errorf("%s: extract needs both variable_name and json_path", where)
			}
		}
	}

	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// CreateFromJSONRequest is the body of POST /load-tests/from-json.
type CreateFromJSONRequest struct {
	Identity              string         `json:"identity"`
	Name                  string         `json:"name"`
	Description           string         `json:"description,omitempty"`
	ToolType              ToolType       `json:"toolType"`
	InfraType             string         `json:"infraType,omitempty"`
	InfraIdentifier       string         `json:"infraIdentifier"`
	EnvironmentIdentifier string         `json:"environmentIdentifier"`
	Tags                  []string       `json:"tags,omitempty"`
	ServiceReferences     []string       `json:"serviceReferences,omitempty"`
	TargetType            string         `json:"targetType,omitempty"`
	JSONSpec              JSONSpec       `json:"jsonSpec"`
	ToolConfig            map[string]any `json:"toolConfig,omitempty"`
	Variables             []Variable     `json:"variables,omitempty"`
}

// UpdateJSONSpecRequest is the body of PUT /load-tests/{identity}/json-script.
// Replacing the spec creates a new script revision, as an upload does.
type UpdateJSONSpecRequest struct {
	JSONSpec    JSONSpec `json:"jsonSpec"`
	Description string   `json:"description,omitempty"`
}

func (c *Client) CreateFromJSON(ctx context.Context, body CreateFromJSONRequest) (*LoadTest, error) {
	out := &LoadTest{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-tests/from-json",
		Body:   body,
	}, out)
	return out, err
}

func (c *Client) UpdateJSONSpec(ctx context.Context, identity string, body UpdateJSONSpecRequest) (*ScriptRevision, error) {
	out := &ScriptRevision{}
	err := c.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/json-script",
		Body:   body,
	}, out)
	return out, err
}
