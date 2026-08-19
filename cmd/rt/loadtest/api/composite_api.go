package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// A composite pairs a load test with a chaos probe so load is applied while a
// fault is injected. The service runs it as a pipeline, hence the shapes below.

// CompositeLoadTestSelection names the load test to drive traffic.
type CompositeLoadTestSelection struct {
	LoadTestRef string `json:"loadTestRef"`
}

// CompositeProbeSelection names the chaos probe to run alongside it. An empty
// InfraReference leaves the pipeline step asking for it as a runtime input.
type CompositeProbeSelection struct {
	Identity       string `json:"identity"`
	InfraReference string `json:"infraReference,omitempty"`
	Duration       string `json:"duration,omitempty"`
}

// CreateCompositeRequest is the body of POST /composite-load-tests. The field
// is "identifier", not the "identity" the rest of this API uses.
type CreateCompositeRequest struct {
	Name        string                     `json:"name"`
	Identifier  string                     `json:"identifier"`
	Description string                     `json:"description,omitempty"`
	Objective   string                     `json:"objective,omitempty"`
	Tags        map[string]string          `json:"tags,omitempty"`
	LoadTest    CompositeLoadTestSelection `json:"loadTest"`
	Probe       CompositeProbeSelection    `json:"probe"`
}

// Composite is the response to creating a composite load test.
type Composite struct {
	PipelineIdentifier string `json:"pipelineIdentifier"`
	StageIdentifier    string `json:"stageIdentifier"`
	Name               string `json:"name"`
	Identifier         string `json:"identifier"`
}

// CompositeExecution summarises one pipeline execution. The timestamps are Unix
// milliseconds, unlike the RFC 3339 strings used on runs.
type CompositeExecution struct {
	ExecutionID string `json:"executionId"`
	Status      string `json:"status"`
	StartTs     int64  `json:"startTs"`
	EndTs       int64  `json:"endTs"`
	TriggerType string `json:"triggerType,omitempty"`
	Username    string `json:"username,omitempty"`
}

// CompositeSummary is one entry in the list response.
type CompositeSummary struct {
	Name               string               `json:"name"`
	PipelineIdentifier string               `json:"pipelineIdentifier"`
	Description        string               `json:"description,omitempty"`
	Tags               map[string]string    `json:"tags,omitempty"`
	StageNames         []string             `json:"stageNames,omitempty"`
	LoadTestCount      int                  `json:"loadTestCount"`
	ProbeCount         int                  `json:"probeCount"`
	CreatedAt          int64                `json:"createdAt"`
	LastUpdatedAt      int64                `json:"lastUpdatedAt"`
	RecentExecutions   []CompositeExecution `json:"recentExecutions,omitempty"`
}

type CompositeList struct {
	Items      []*CompositeSummary `json:"items"`
	Pagination Pagination          `json:"pagination"`
}

// CompositeSortFields are pipeline-service field names and differ from the ones
// "list" takes, so a value valid there is rejected here.
var CompositeSortFields = []string{"name", "lastUpdatedAt", "executionSummaryInfo.lastExecutionTs"}

func (c *Client) CreateComposite(ctx context.Context, body CreateCompositeRequest) (*Composite, error) {
	out := &Composite{}
	err := c.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/composite-load-tests",
		Body:   body,
	}, out)
	return out, err
}

func (c *Client) ListComposites(ctx context.Context, opts ListOptions) (*CompositeList, error) {
	query := opts.values()
	// The composite list is backed by pipeline service and has no toolType,
	// environment or tag filter; sending them would be silently ignored.
	query.Del("toolType")
	query.Del("environmentIdentifier")
	query.Del("status")
	query.Del("tags")

	out := &CompositeList{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/composite-load-tests",
		Query:  query,
	}, out)
	return out, err
}

// UsageWindow is the range a usage query covers, in Unix milliseconds, both
// ends required. Figures are account-wide; an org or project does not narrow them.
type UsageWindow struct {
	StartMillis int64
	EndMillis   int64
}

func (w UsageWindow) values() url.Values {
	v := url.Values{}
	v.Set("startTime", strconv.FormatInt(w.StartMillis, 10))
	v.Set("endTime", strconv.FormatInt(w.EndMillis, 10))
	return v
}

// ServiceUsage is one service's consumption over the window. Complexity is the
// weighted figure the account is billed on.
type ServiceUsage struct {
	ServiceID        string   `json:"serviceId"`
	OrgID            string   `json:"orgId"`
	ProjectID        string   `json:"projectId"`
	RunCount         int64    `json:"runCount"`
	PeakUsers        int      `json:"peakUsers"`
	MaxWorkerCount   int      `json:"maxWorkerCount"`
	TotalDurationSec int64    `json:"totalDurationSec"`
	TotalVUSeconds   int64    `json:"totalVuSeconds"`
	ToolTypes        []string `json:"toolTypes"`
	InfraTypes       []string `json:"infraTypes"`
	Complexity       float64  `json:"complexity"`
}

type UsageDetails struct {
	AccountID string         `json:"accountId"`
	Services  []ServiceUsage `json:"services"`
	Total     int            `json:"total"`
}

// RatioConfig is the weighting applied to raw counters to reach a complexity
// figure.
type RatioConfig struct {
	AccountID          string                        `json:"accountId"`
	BaseServiceWeight  float64                       `json:"baseServiceWeight"`
	MetricWeights      map[string]float64            `json:"metricWeights"`
	MetricThresholds   map[string]float64            `json:"metricThresholds,omitempty"`
	CategoricalWeights map[string]map[string]float64 `json:"categoricalWeights,omitempty"`
	UpdatedBy          string                        `json:"updatedBy,omitempty"`
	UpdatedAt          int64                         `json:"updatedAt,omitempty"`
}

type OverallUsage struct {
	AccountID    string      `json:"accountId"`
	RatioConfig  RatioConfig `json:"ratioConfig"`
	TotalUsage   float64     `json:"totalUsage"`
	ServiceCount int         `json:"serviceCount"`
}

func (c *Client) GetUsageDetails(ctx context.Context, window UsageWindow) (*UsageDetails, error) {
	out := &UsageDetails{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-service-usage/details",
		Query:  window.values(),
	}, out)
	return out, err
}

func (c *Client) GetOverallUsage(ctx context.Context, window UsageWindow) (*OverallUsage, error) {
	out := &OverallUsage{}
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-service-usage/overall",
		Query:  window.values(),
	}, out)
	return out, err
}

// GetUsageReport returns rows, the first a header and the last a total. A
// matrix rather than an object, so it can be written straight out as CSV.
func (c *Client) GetUsageReport(ctx context.Context, window UsageWindow) ([][]string, error) {
	var out [][]string
	err := c.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/load-service-usage/report",
		Query:  window.values(),
	}, &out)
	return out, err
}
