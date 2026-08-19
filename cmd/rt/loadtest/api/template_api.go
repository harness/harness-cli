package api

import (
	"context"
	"net/http"
	"net/url"
)

// ImportType selects how a template is attached to a load test. A REFERENCE
// import stays pinned and can be re-synced; a LOCAL import is a one-time copy.
type ImportType string

const (
	ImportReference ImportType = "REFERENCE"
	ImportLocal     ImportType = "LOCAL"
)

var ImportTypes = []ImportType{ImportReference, ImportLocal}

// TemplateRef selects the template to import. The org and project fields let a
// test import from a wider scope than its own.
type TemplateRef struct {
	Identity               string `json:"identity"`
	Revision               string `json:"revision"`
	HubIdentity            string `json:"hubIdentity"`
	OrganizationIdentifier string `json:"organizationIdentifier,omitempty"`
	ProjectIdentifier      string `json:"projectIdentifier,omitempty"`
}

// CreateFromTemplateRequest is the body of POST /load-tests/from-template.
// Infrastructure and environment must be concrete even if the template is not.
type CreateFromTemplateRequest struct {
	Identity              string         `json:"identity"`
	Name                  string         `json:"name"`
	Description           string         `json:"description,omitempty"`
	TemplateReference     TemplateRef    `json:"templateReference"`
	ImportType            ImportType     `json:"importType"`
	InfraIdentifier       string         `json:"infraIdentifier"`
	EnvironmentIdentifier string         `json:"environmentIdentifier"`
	TargetType            string         `json:"targetType,omitempty"`
	Variables             []Variable     `json:"variables,omitempty"`
	ToolConfig            map[string]any `json:"toolConfig,omitempty"`
	ServiceReferences     []string       `json:"serviceReferences,omitempty"`
}

func (c *Client) CreateFromTemplate(ctx context.Context, body CreateFromTemplateRequest) (*LoadTest, error) {
	out := &LoadTest{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-tests/from-template",
		Body:   body,
	}, out)
	return out, err
}

// SyncTemplate re-pins a REFERENCE import to the newest template revision.
func (c *Client) SyncTemplate(ctx context.Context, identity string) (*LoadTest, error) {
	out := &LoadTest{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-tests/" + url.PathEscape(identity) + "/sync-template",
	}, out)
	return out, err
}

// hubQuery sends an empty hub as an empty value rather than omitting it, which
// is what these endpoints expect. They match hubIdentity exactly, so an empty
// one selects a template filed under no hub rather than a default hub. The list
// endpoint is the exception and applies the filter only when it is non-empty.
func hubQuery(hubIdentity string) url.Values {
	query := url.Values{}
	query.Set("hubIdentity", hubIdentity)
	return query
}

// Template is one revision of a stored template. There is no separate revision
// type: the revision endpoints return this same envelope.
type Template struct {
	UniqueID               string             `json:"uniqueId"`
	Identity               string             `json:"identity"`
	Revision               string             `json:"revision"`
	Name                   string             `json:"name"`
	Description            string             `json:"description,omitempty"`
	ToolType               ToolType           `json:"toolType"`
	InfraType              string             `json:"infraType,omitempty"`
	HubIdentity            string             `json:"hubIdentity,omitempty"`
	AccountIdentifier      string             `json:"accountIdentifier"`
	OrganizationIdentifier string             `json:"organizationIdentifier,omitempty"`
	ProjectIdentifier      string             `json:"projectIdentifier,omitempty"`
	ParentUniqueID         string             `json:"parentUniqueId,omitempty"`
	Tags                   []string           `json:"tags,omitempty"`
	ScriptSource           string             `json:"scriptSource,omitempty"`
	ScriptContent          string             `json:"scriptContent,omitempty"`
	InfraID                string             `json:"infraId,omitempty"`
	EnvID                  string             `json:"envId,omitempty"`
	ToolConfig             map[string]any     `json:"toolConfig,omitempty"`
	DetectedVariables      []DetectedVariable `json:"detectedVariables,omitempty"`
	YAML                   string             `json:"yaml,omitempty"`
	CreatedAt              string             `json:"createdAt"`
	CreatedBy              string             `json:"createdBy"`
	UpdatedAt              string             `json:"updatedAt"`
	UpdatedBy              string             `json:"updatedBy"`
}

// DetectedVariable is an environment variable the service found by scanning
// the script, as opposed to one the author declared.
type DetectedVariable struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Type        string `json:"type,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type TemplateList struct {
	Items      []*Template `json:"items"`
	Pagination Pagination  `json:"pagination"`
}

// TemplateVariables is the response of the template variables endpoint, the
// counterpart of VariablesResponse for a template revision.
type TemplateVariables struct {
	TemplateIdentity string     `json:"templateIdentity"`
	TemplateRevision string     `json:"templateRevision"`
	Variables        []Variable `json:"variables,omitempty"`
	Inputs           []Input    `json:"inputs"`
}

// CreateTemplateRequest publishes a template and its first revision together,
// so Revision is required. The hub travels as a query parameter, not here.
type CreateTemplateRequest struct {
	Identity    string         `json:"identity"`
	Revision    string         `json:"revision"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	ToolType    ToolType       `json:"toolType"`
	InfraType   string         `json:"infraType,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	InfraID     string         `json:"infraId,omitempty"`
	EnvID       string         `json:"envId,omitempty"`
	Variables   []Variable     `json:"variables,omitempty"`
	ToolConfig  map[string]any `json:"toolConfig,omitempty"`
}

// CreateRevisionRequest inherits every empty field from the latest revision.
// Variables is the exception: a non-nil slice replaces, and an empty one clears.
type CreateRevisionRequest struct {
	Revision    string         `json:"revision"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	InfraID     string         `json:"infraId,omitempty"`
	EnvID       string         `json:"envId,omitempty"`
	Variables   []Variable     `json:"variables,omitempty"`
	ToolConfig  map[string]any `json:"toolConfig,omitempty"`
}

// UpdateTemplateRequest edits the latest revision in place. It cannot change
// the script; publish a new revision for that.
type UpdateTemplateRequest struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	InfraType   string         `json:"infraType,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	InfraID     string         `json:"infraId,omitempty"`
	EnvID       string         `json:"envId,omitempty"`
	Variables   []Variable     `json:"variables,omitempty"`
	ToolConfig  map[string]any `json:"toolConfig,omitempty"`
}

func (c *Client) CreateTemplate(ctx context.Context, hubIdentity string, body CreateTemplateRequest) (*Template, error) {
	out := &Template{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-test-templates",
		Query:  hubQuery(hubIdentity),
		Body:   body,
	}, out)
	return out, err
}

// TemplateListOptions takes InfraType, which the load test list does not, and
// has no tag or environment filter.
type TemplateListOptions struct {
	ListOptions
	HubIdentity string
	InfraType   string
}

func (o TemplateListOptions) values() url.Values {
	query := o.ListOptions.values()
	// The template store filters on hub, tool and infra type only; the load
	// test filters below have no equivalent and would be silently ignored.
	query.Del("environmentIdentifier")
	query.Del("status")
	query.Del("tags")

	query.Set("hubIdentity", o.HubIdentity)
	if o.InfraType != "" {
		query.Set("infraType", o.InfraType)
	}
	return query
}

func (c *Client) ListTemplates(ctx context.Context, opts TemplateListOptions) (*TemplateList, error) {
	out := &TemplateList{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-test-templates",
		Query:  opts.values(),
	}, out)
	return out, err
}

// GetTemplate returns the latest revision of a template. Use
// GetTemplateRevision for a specific one.
func (c *Client) GetTemplate(ctx context.Context, hubIdentity, identity string) (*Template, error) {
	out := &Template{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-test-templates/" + url.PathEscape(identity),
		Query:  hubQuery(hubIdentity),
	}, out)
	return out, err
}

func (c *Client) UpdateTemplate(ctx context.Context, hubIdentity, identity string, body UpdateTemplateRequest) (*Template, error) {
	out := &Template{}
	err := c.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/load-test-templates/" + url.PathEscape(identity),
		Query:  hubQuery(hubIdentity),
		Body:   body,
	}, out)
	return out, err
}

// DeleteTemplate removes a template and every one of its revisions.
func (c *Client) DeleteTemplate(ctx context.Context, hubIdentity, identity string) error {
	return c.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/load-test-templates/" + url.PathEscape(identity),
		Query:  hubQuery(hubIdentity),
	}, nil)
}

// ListTemplateRevisions returns every revision of a template, newest first.
func (c *Client) ListTemplateRevisions(ctx context.Context, hubIdentity, identity string) ([]*Template, error) {
	var out []*Template
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-test-templates/" + url.PathEscape(identity) + "/revisions",
		Query:  hubQuery(hubIdentity),
	}, &out)
	return out, err
}

func (c *Client) GetTemplateRevision(ctx context.Context, hubIdentity, identity, revision string) (*Template, error) {
	out := &Template{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-test-templates/" + url.PathEscape(identity) + "/revisions/" + url.PathEscape(revision),
		Query:  hubQuery(hubIdentity),
	}, out)
	return out, err
}

// CreateTemplateRevision publishes a new immutable revision of a template.
func (c *Client) CreateTemplateRevision(ctx context.Context, hubIdentity, identity string, body CreateRevisionRequest) (*Template, error) {
	out := &Template{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/load-test-templates/" + url.PathEscape(identity) + "/revisions",
		Query:  hubQuery(hubIdentity),
		Body:   body,
	}, out)
	return out, err
}

func (c *Client) DeleteTemplateRevision(ctx context.Context, hubIdentity, identity, revision string) error {
	return c.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/load-test-templates/" + url.PathEscape(identity) + "/revisions/" + url.PathEscape(revision),
		Query:  hubQuery(hubIdentity),
	}, nil)
}

// ExportTemplateYAML returns the template as YAML, which is not JSON and so is
// returned undecoded.
func (c *Client) ExportTemplateYAML(ctx context.Context, hubIdentity, identity string) ([]byte, error) {
	return c.doRaw(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-test-templates/" + url.PathEscape(identity) + "/yaml",
		Query:  hubQuery(hubIdentity),
	})
}

// GetTemplateVariables lists a revision's variables and the runtime inputs
// discovered in its tool configuration. An empty revision resolves the latest.
func (c *Client) GetTemplateVariables(ctx context.Context, hubIdentity, identity, revision string) (*TemplateVariables, error) {
	query := hubQuery(hubIdentity)
	if revision != "" {
		query.Set("revision", revision)
	}

	out := &TemplateVariables{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-test-templates/" + url.PathEscape(identity) + "/variables",
		Query:  query,
	}, out)
	return out, err
}
