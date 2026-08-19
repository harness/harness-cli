package api

import (
	"fmt"
	"strings"
)

// ToolType selects the load testing engine.
type ToolType string

const (
	ToolJMeter ToolType = "JMeter"
	ToolLocust ToolType = "Locust"
	ToolK6     ToolType = "K6"
)

// SupportedToolTypes are the tool types accepted for new load tests.
var SupportedToolTypes = []ToolType{ToolJMeter, ToolLocust, ToolK6}

// ToolKey is the toolConfig key a tool's configuration nests under, for
// example toolConfig.jmeter. Returns "" for an unrecognised tool.
func ToolKey(t ToolType) string {
	switch t {
	case ToolJMeter:
		return "jmeter"
	case ToolLocust:
		return "locust"
	case ToolK6:
		return "k6"
	}
	return ""
}

// Mode selects how a test is defined. Every engine supports script and image;
// k6's legacy ui mode was retired with no replacement.
type Mode string

const (
	ModeScript Mode = "script"
	ModeImage  Mode = "image"
)

// ScriptSource records where a script artifact comes from.
type ScriptSource string

const (
	SourceInline ScriptSource = "inline"
	SourceImage  ScriptSource = "image"
)

// TargetType is the infrastructure family the test runs on. K6 is only
// supported on kubernetes.
type TargetType string

const (
	TargetKubernetes TargetType = "kubernetes"
	TargetLinux      TargetType = "linux"
)

// CleanupPolicy controls whether a run's Kubernetes resources are removed when
// it finishes.
type CleanupPolicy string

const (
	CleanupDelete CleanupPolicy = "delete"
	CleanupRetain CleanupPolicy = "retain"
)

// RuntimeInput marks a leaf as supplied at run time. Any leaf of toolConfig
// may carry it, and --runtime-value binds it by path.
const RuntimeInput = "<+input>"

// ResourceList maps "cpu" or "memory" to a Kubernetes quantity such as "500m"
// or "512Mi".
type ResourceList map[string]string

type ResourceRequirements struct {
	Limits   ResourceList `json:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty"`
}

// EnvVar is one environment variable forwarded to the runner. A secret entry
// carries a reference resolved at dispatch rather than a literal value.
type EnvVar struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

// Variable is a user-defined value referenced from config as
// <+variable.NAME>. Variables live beside toolConfig, not inside it.
type Variable struct {
	Name        string `json:"name"`
	Value       any    `json:"value"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// JMeterSpec is the toolConfig.jmeter block. The specs here generate skeletons
// and validate paths; the wire type is an opaque map, so "<+input>" survives.
type JMeterSpec struct {
	Mode       Mode             `json:"mode"`
	Script     ScriptSpec       `json:"script"`
	Tunables   Tunables         `json:"tunables"`
	Properties []JMeterProperty `json:"properties,omitempty"`
	EnvVars    []EnvVar         `json:"envVars,omitempty"`
	Thresholds []Threshold      `json:"thresholds,omitempty"`
}

// LocustSpec is the toolConfig.locust block. Locust has no engine-specific
// extension beyond the shared blocks.
type LocustSpec struct {
	Mode     Mode       `json:"mode"`
	Script   ScriptSpec `json:"script"`
	Tunables Tunables   `json:"tunables"`
	EnvVars  []EnvVar   `json:"envVars,omitempty"`
}

// K6Spec is the toolConfig.k6 block.
type K6Spec struct {
	Mode       Mode              `json:"mode"`
	Script     ScriptSpec        `json:"script"`
	Tunables   Tunables          `json:"tunables"`
	Options    *K6Options        `json:"options,omitempty"`
	MetricTags map[string]string `json:"metricTags,omitempty"`
	EnvVars    []EnvVar          `json:"envVars,omitempty"`
	Thresholds []Threshold       `json:"thresholds,omitempty"`
}

// ScriptSpec is the shared artifact block. Content and testPlan apply in
// script mode; image, entrypoint, runArgs and imagePullSecret in image mode.
type ScriptSpec struct {
	Source ScriptSource `json:"source,omitempty"`
	// Content is the base64-encoded script body.
	Content string `json:"content,omitempty"`
	// TestPlan names the main .jmx inside a JMeter .zip workspace.
	TestPlan        string   `json:"testPlan,omitempty"`
	Image           string   `json:"image,omitempty"`
	Entrypoint      string   `json:"entrypoint,omitempty"`
	RunArgs         []string `json:"runArgs,omitempty"`
	ImagePullSecret string   `json:"imagePullSecret,omitempty"`
}

// Tunables is the load shape, shared by every engine. Fields are any because
// each may hold "<+input>" or "<+variable.NAME>" instead of its natural type.
type Tunables struct {
	TargetURL       any `json:"targetUrl,omitempty"`
	TargetUsers     any `json:"targetUsers,omitempty"`
	RampUpTimeSec   any `json:"rampUpTimeSec,omitempty"`
	DurationSeconds any `json:"durationSeconds,omitempty"`
	// WorkerCount above one selects a distributed controller and injector
	// topology; one or less runs a single pod.
	WorkerCount any `json:"workerCount,omitempty"`
	// MaxDurationSec caps any run of this test, as an operator guardrail.
	MaxDurationSec any `json:"maxDurationSec,omitempty"`
	// SpawnRate is users started per second. Locust only.
	SpawnRate any `json:"spawnRate,omitempty"`
	// HostURL is forwarded to the script as __ENV.HOST_URL. K6 only.
	HostURL any `json:"hostUrl,omitempty"`
	// Iterations caps the run; durationSeconds wins when both are set.
	Iterations any `json:"iterations,omitempty"`
	RPSLimit   any `json:"rpsLimit,omitempty"`
}

// JMeterProperty is a runtime property override passed to the plan.
type JMeterProperty struct {
	Key           string `json:"key"`
	Value         any    `json:"value"`
	SendToEngines bool   `json:"sendToEngines,omitempty"`
}

// Threshold is one aggregate pass/fail criterion. Under the unified schema
// thresholds are a first-class block for both JMeter and k6.
type Threshold struct {
	Metric      string `json:"metric"`
	Stat        string `json:"stat,omitempty"`
	Operator    string `json:"operator"`
	Value       any    `json:"value"`
	AbortOnFail bool   `json:"abortOnFail,omitempty"`
}

// JMeterThresholdMetrics are the metrics a JMeter threshold can assert on.
var JMeterThresholdMetrics = []string{"response_time_ms", "error_rate_pct", "throughput_rps", "latency_ms"}

// ThresholdOperators are the comparison operators accepted by thresholds.
var ThresholdOperators = []string{"<", "<=", ">", ">=", "==", "!="}

// K6Options is advanced k6 HTTP and runtime tuning. Zero values mean the
// runner default applies.
type K6Options struct {
	UserAgent             string `json:"userAgent,omitempty"`
	DNSStrategy           string `json:"dnsStrategy,omitempty"`
	NoConnectionReuse     bool   `json:"noConnectionReuse,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
	BatchConcurrent       int    `json:"batchConcurrent,omitempty"`
	BatchPerHost          int    `json:"batchPerHost,omitempty"`
	DiscardResponseBodies bool   `json:"discardResponseBodies,omitempty"`
	RPSLimit              int    `json:"rpsLimit,omitempty"`
}

// SetPath writes value at a dotted path, creating intermediate maps as it goes.
// Flag values are merged onto an opaque toolConfig this way.
func SetPath(root map[string]any, path string, value any) error {
	segments := strings.Split(path, ".")
	if len(segments) == 0 || segments[0] == "" {
		return fmt.Errorf("empty config path")
	}

	node := root
	for i, segment := range segments[:len(segments)-1] {
		child, present := node[segment]
		if !present || child == nil {
			next := map[string]any{}
			node[segment] = next
			node = next
			continue
		}
		next, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("cannot set %q: %q is a value, not a block", path, strings.Join(segments[:i+1], "."))
		}
		node = next
	}

	node[segments[len(segments)-1]] = value
	return nil
}

// GetPath reads the value at a dotted path. The second return is false when
// any segment is missing.
func GetPath(root map[string]any, path string) (any, bool) {
	var current any = root
	for _, segment := range strings.Split(path, ".") {
		node, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = node[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
