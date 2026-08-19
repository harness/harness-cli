package api

// RunStatus is the lifecycle state of a load test run.
type RunStatus string

const (
	RunPending  RunStatus = "Pending"
	RunRunning  RunStatus = "Running"
	RunStopping RunStatus = "Stopping"
	RunStopped  RunStatus = "Stopped"
	RunFinished RunStatus = "Finished"
	RunFailed   RunStatus = "Failed"
)

// Terminal reports whether no further status change is expected, which is the
// condition the watch command polls for.
func (s RunStatus) Terminal() bool {
	switch s {
	case RunStopped, RunFinished, RunFailed:
		return true
	}
	return false
}

// CreateRequest is the body of POST /load-tests. ToolConfig stays an opaque
// map: any leaf may hold an expression a typed int or float could not.
type CreateRequest struct {
	Identity              string   `json:"identity"`
	Name                  string   `json:"name"`
	Description           string   `json:"description,omitempty"`
	ToolType              ToolType `json:"toolType"`
	InfraType             string   `json:"infraType,omitempty"`
	InfraIdentifier       string   `json:"infraIdentifier"`
	EnvironmentIdentifier string   `json:"environmentIdentifier"`
	Tags                  []string `json:"tags,omitempty"`
	// ServiceReferences links the test to chaos services, which are the
	// licensed unit. At least one is required unless UseSampleTest is set.
	ServiceReferences []string              `json:"serviceReferences,omitempty"`
	TargetType        string                `json:"targetType,omitempty"`
	ToolConfig        map[string]any        `json:"toolConfig,omitempty"`
	Variables         []Variable            `json:"variables,omitempty"`
	UseSampleTest     bool                  `json:"useSampleTest,omitempty"`
	CleanupPolicy     CleanupPolicy         `json:"cleanupPolicy,omitempty"`
	Resources         *ResourceRequirements `json:"resources,omitempty"`
}

// UpdateRequest is the body of PUT /load-tests/{identity}. Every field is
// optional; omitted fields keep their stored value.
type UpdateRequest struct {
	Name                  string                `json:"name,omitempty"`
	Description           string                `json:"description,omitempty"`
	Tags                  []string              `json:"tags,omitempty"`
	ServiceReferences     []string              `json:"serviceReferences,omitempty"`
	InfraIdentifier       string                `json:"infraIdentifier,omitempty"`
	EnvironmentIdentifier string                `json:"environmentIdentifier,omitempty"`
	TargetType            string                `json:"targetType,omitempty"`
	ToolConfig            map[string]any        `json:"toolConfig,omitempty"`
	Variables             []Variable            `json:"variables,omitempty"`
	CleanupPolicy         CleanupPolicy         `json:"cleanupPolicy,omitempty"`
	Resources             *ResourceRequirements `json:"resources,omitempty"`
}

// UserDetails is the decoded actor on created/updated fields.
type UserDetails struct {
	UserID   string `json:"userID"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// LoadTest is the API view of a stored load test.
type LoadTest struct {
	UniqueID               string   `json:"uniqueId"`
	Identity               string   `json:"identity"`
	Name                   string   `json:"name"`
	Description            string   `json:"description,omitempty"`
	ToolType               ToolType `json:"toolType"`
	InfraType              string   `json:"infraType,omitempty"`
	Tags                   []string `json:"tags,omitempty"`
	ServiceReferences      []string `json:"serviceReferences,omitempty"`
	AccountIdentifier      string   `json:"accountIdentifier"`
	OrganizationIdentifier string   `json:"organizationIdentifier"`
	ProjectIdentifier      string   `json:"projectIdentifier"`
	InfraIdentifier        string   `json:"infraIdentifier,omitempty"`
	EnvironmentIdentifier  string   `json:"environmentIdentifier,omitempty"`
	TargetType             string   `json:"targetType,omitempty"`
	IsSampleTest           bool     `json:"isSampleTest"`
	// TargetUsers and DurationSeconds are display projections of the tool
	// config tunables and are returned as strings.
	TargetUsers              string                `json:"targetUsers,omitempty"`
	DurationSeconds          string                `json:"durationSeconds,omitempty"`
	MaxDurationSec           *int                  `json:"maxDurationSec,omitempty"`
	CleanupPolicy            CleanupPolicy         `json:"cleanupPolicy,omitempty"`
	Resources                *ResourceRequirements `json:"resources,omitempty"`
	ToolConfig               map[string]any        `json:"toolConfig,omitempty"`
	Variables                []Variable            `json:"variables,omitempty"`
	LatestRevisionIdentifier string                `json:"latestRevisionIdentifier,omitempty"`
	RecentRuns               []RecentRun           `json:"recentRuns,omitempty"`
	LastExecuted             string                `json:"lastExecuted,omitempty"`
	CreatedAt                string                `json:"createdAt"`
	CreatedBy                string                `json:"createdBy"`
	UpdatedAt                string                `json:"updatedAt"`
	UpdatedBy                string                `json:"updatedBy"`
	CreatedByUserDetails     *UserDetails          `json:"createdByUserDetails,omitempty"`
	UpdatedByUserDetails     *UserDetails          `json:"updatedByUserDetails,omitempty"`
	TemplateReference        *TemplateReference    `json:"templateReference,omitempty"`
	ImportType               string                `json:"importType,omitempty"`
	TemplateUpdateAvailable  bool                  `json:"templateUpdateAvailable,omitempty"`
	YAML                     string                `json:"yaml,omitempty"`
}

// TemplateReference is the pinned template a test was imported from.
type TemplateReference struct {
	Identity string `json:"identity,omitempty"`
	Name     string `json:"name,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// RecentRun is the run summary embedded in a load test listing.
type RecentRun struct {
	UniqueID        string    `json:"uniqueId"`
	Identity        string    `json:"identity"`
	Name            string    `json:"name,omitempty"`
	Status          RunStatus `json:"status"`
	TargetUsers     int       `json:"targetUsers"`
	SpawnRate       float64   `json:"spawnRate"`
	DurationSeconds *int      `json:"durationSeconds,omitempty"`
	StartedAt       string    `json:"startedAt,omitempty"`
	FinishedAt      string    `json:"finishedAt,omitempty"`
	CreatedAt       string    `json:"createdAt"`
	CreatedBy       string    `json:"createdBy"`
}

type Pagination struct {
	Index      int64 `json:"index"`
	Limit      int64 `json:"limit"`
	TotalPages int64 `json:"totalPages"`
	TotalItems int64 `json:"totalItems"`
}

type LoadTestList struct {
	Items      []*LoadTest `json:"items"`
	Pagination Pagination  `json:"pagination"`
}

type RunList struct {
	Items      []*Run     `json:"items"`
	Pagination Pagination `json:"pagination"`
}

// RunValue overrides one named variable or input for a single run.
type RunValue struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// CreateRunRequest is the body of POST /loadtests/{identity}/runs.
type CreateRunRequest struct {
	Identity string     `json:"identity"`
	Name     string     `json:"name,omitempty"`
	Values   []RunValue `json:"values,omitempty"`
	// RuntimeValues overrides toolConfig leaves authored as "<+input>",
	// keyed by dotted path such as "jmeter.tunables.targetUsers".
	RuntimeValues map[string]any `json:"runtimeValues,omitempty"`
}

// UpdateRunRequest retunes a run that is already in flight. The scope
// identifiers are part of the body for this endpoint, not just the query.
type UpdateRunRequest struct {
	Identity               string   `json:"identity"`
	AccountIdentifier      string   `json:"accountIdentifier"`
	OrganizationIdentifier string   `json:"organizationIdentifier"`
	ProjectIdentifier      string   `json:"projectIdentifier"`
	TargetUsers            *int     `json:"targetUsers,omitempty"`
	SpawnRate              *float64 `json:"spawnRate,omitempty"`
}

// Run is the API view of a single load test run.
type Run struct {
	UniqueID               string    `json:"uniqueId"`
	NotifyID               string    `json:"notifyId,omitempty"`
	Identity               string    `json:"identity"`
	LoadTestIdentity       string    `json:"loadTestIdentity"`
	LoadTestName           string    `json:"loadTestName,omitempty"`
	Name                   string    `json:"name,omitempty"`
	RunSequence            int       `json:"runSequence"`
	AccountIdentifier      string    `json:"accountIdentifier"`
	OrganizationIdentifier string    `json:"organizationIdentifier"`
	ProjectIdentifier      string    `json:"projectIdentifier"`
	EnvironmentIdentifier  string    `json:"environmentIdentifier,omitempty"`
	InfraIdentifier        string    `json:"infraIdentifier"`
	TargetType             string    `json:"targetType,omitempty"`
	ToolType               ToolType  `json:"toolType"`
	TargetUsers            int       `json:"targetUsers"`
	SpawnRate              float64   `json:"spawnRate"`
	RampUpTimeSec          *int      `json:"rampUpTimeSec,omitempty"`
	DurationSeconds        *int      `json:"durationSeconds"`
	WorkerCount            int       `json:"workerCount"`
	ScriptSource           string    `json:"scriptSource,omitempty"`
	ScriptImage            string    `json:"scriptImage,omitempty"`
	ScriptEntrypoint       string    `json:"scriptEntrypoint,omitempty"`
	ScriptImagePullSecret  string    `json:"scriptImagePullSecret,omitempty"`
	RunArgs                []string  `json:"runArgs,omitempty"`
	LogStreamID            string    `json:"logStreamId,omitempty"`
	Status                 RunStatus `json:"status"`
	Action                 string    `json:"action,omitempty"`

	ResolvedVariables     map[string]any `json:"resolvedVariables,omitempty"`
	ResolvedRuntimeValues map[string]any `json:"resolvedRuntimeValues,omitempty"`

	StartedAt            *string         `json:"startedAt,omitempty"`
	FinishedAt           *string         `json:"finishedAt,omitempty"`
	CreatedAt            string          `json:"createdAt"`
	CreatedBy            string          `json:"createdBy"`
	UpdatedAt            string          `json:"updatedAt"`
	UpdatedBy            string          `json:"updatedBy"`
	CreatedByUserDetails *UserDetails    `json:"createdByUserDetails,omitempty"`
	LastMetrics          *MetricSnapshot `json:"lastMetrics,omitempty"`
	ErrorCode            string          `json:"errorCode,omitempty"`
	ErrorMessage         string          `json:"errorMessage,omitempty"`
	Errors               []RunError      `json:"errors,omitempty"`
}

type RunError struct {
	Timestamp    string `json:"timestamp"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	Source       string `json:"source,omitempty"`
	Phase        string `json:"phase,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Context      string `json:"context,omitempty"`
}

// MetricSnapshot is one point-in-time aggregate across all injectors.
type MetricSnapshot struct {
	Timestamp         string              `json:"timestamp"`
	TotalRPS          float64             `json:"totalRps"`
	TotalRequests     int64               `json:"totalRequests"`
	TotalFailures     int64               `json:"totalFailures"`
	ErrorRate         float64             `json:"errorRate"`
	AverageResponseMs float64             `json:"avgResponseMs"`
	P50ResponseMs     float64             `json:"p50ResponseMs"`
	P95ResponseMs     float64             `json:"p95ResponseMs"`
	P99ResponseMs     float64             `json:"p99ResponseMs"`
	CurrentUsers      int                 `json:"currentUsers"`
	RequestStats      map[string]*ReqStat `json:"requestStats,omitempty"`
	// Latency percentiles measure time to first byte and are JMeter only.
	AvgLatencyMs float64 `json:"avgLatencyMs,omitempty"`
	P50LatencyMs float64 `json:"p50LatencyMs,omitempty"`
	P95LatencyMs float64 `json:"p95LatencyMs,omitempty"`
	P99LatencyMs float64 `json:"p99LatencyMs,omitempty"`
}

// ReqStat is per-endpoint statistics. Locust leaves the percentile and
// throughput fields zero.
type ReqStat struct {
	Method                string  `json:"method"`
	Name                  string  `json:"name"`
	NumRequests           int64   `json:"numRequests"`
	NumFailures           int64   `json:"numFailures"`
	AvgResponseTime       float64 `json:"avgResponseTime"`
	MinResponseTime       float64 `json:"minResponseTime"`
	MaxResponseTime       float64 `json:"maxResponseTime"`
	MedianResponseTime    float64 `json:"medianResponseTime"`
	P95ResponseMs         float64 `json:"p95ResponseMs"`
	P99ResponseMs         float64 `json:"p99ResponseMs"`
	RequestsPerSec        float64 `json:"requestsPerSec"`
	AvgSizeBytes          float64 `json:"avgSizeBytes"`
	CurrentFailuresPerSec float64 `json:"currentFailuresPerSec"`
}

// UpdateScriptRequest replaces the stored script and creates a revision.
type UpdateScriptRequest struct {
	ScriptContent string `json:"scriptContent"`
	Description   string `json:"description,omitempty"`
}

// ScriptRevision is one stored version of a test's script.
type ScriptRevision struct {
	Identity         string `json:"identity"`
	LoadTestIdentity string `json:"loadTestIdentity"`
	RevisionNumber   int    `json:"revisionNumber"`
	ScriptContent    string `json:"scriptContent"`
	Description      string `json:"description,omitempty"`
	CreatedAt        string `json:"createdAt"`
	CreatedBy        string `json:"createdBy"`
	// IsBundle marks a zip workspace, whose ScriptContent is binary. The
	// bundle fields below carry a readable view instead.
	IsBundle          bool         `json:"isBundle,omitempty"`
	BundleFiles       []BundleFile `json:"bundleFiles,omitempty"`
	BundleMainFile    string       `json:"bundleMainFile,omitempty"`
	BundleMainContent string       `json:"bundleMainContent,omitempty"`
}

type BundleFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// VariablesResponse separates the two: a Variable is declared on the test and
// named, an Input is a "<+input>" leaf of toolConfig addressed by its path.
type VariablesResponse struct {
	LoadTestIdentity string     `json:"loadTestIdentity"`
	TemplateIdentity string     `json:"templateIdentity,omitempty"`
	TemplateRevision string     `json:"templateRevision,omitempty"`
	Variables        []Variable `json:"variables,omitempty"`
	Inputs           []Input    `json:"inputs"`
}

// Input is a "<+input>" leaf supplied at run time. Path is absolute and names
// the tool, ".toolConfig.k6.tunables.targetUsers", the only spelling accepted.
type Input struct {
	Name          string   `json:"name"`
	Value         any      `json:"value"`
	Path          string   `json:"path"`
	Type          string   `json:"type,omitempty"`
	Category      string   `json:"category,omitempty"`
	Description   string   `json:"description,omitempty"`
	Required      bool     `json:"required,omitempty"`
	Default       any      `json:"default,omitempty"`
	AllowedValues []any    `json:"allowedValues,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Validator     string   `json:"validator,omitempty"`
}
