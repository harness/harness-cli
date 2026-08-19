package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// toolWriter edits the toolConfig block of one tool. The block stays an opaque
// map because any leaf may hold an expression instead of its natural type.
type toolWriter struct {
	root map[string]any
	// key is the toolConfig key of the active tool, for example "jmeter".
	key      string
	toolType api.ToolType
}

// newToolWriter prepares root for edits to toolType's block and drops the
// others: a test runs one engine, and the API rejects a stale sibling block.
func newToolWriter(root map[string]any, toolType api.ToolType) (*toolWriter, error) {
	key := api.ToolKey(toolType)
	if key == "" {
		return nil, fmt.Errorf("unsupported --tool-type %q: supported values are %s", toolType, joinTools(api.SupportedToolTypes))
	}

	liftFlattenedToolConfig(root, key)

	for _, other := range api.SupportedToolTypes {
		if other != toolType {
			delete(root, api.ToolKey(other))
		}
	}
	if _, present := root[key].(map[string]any); !present {
		root[key] = map[string]any{}
	}

	return &toolWriter{root: root, key: key, toolType: toolType}, nil
}

// Seeing these at the root of a toolConfig means it was serialised without its
// tool wrapper.
var toolBlockFields = []string{"mode", "tunables", "script", "envVars", "options", "properties"}

// liftFlattenedToolConfig nests a toolConfig the service returned flattened.
// Left alone, a GET-before-write patch lands in a sibling the service ignores.
func liftFlattenedToolConfig(root map[string]any, key string) {
	if _, nested := root[key].(map[string]any); nested {
		return
	}
	for _, other := range api.SupportedToolTypes {
		if _, present := root[api.ToolKey(other)].(map[string]any); present {
			return // nested under some other tool; leave it for the caller to strip
		}
	}

	flattened := false
	for _, field := range toolBlockFields {
		if _, present := root[field]; present {
			flattened = true
			break
		}
	}
	if !flattened {
		return
	}

	block := map[string]any{}
	for name, value := range root {
		block[name] = value
		delete(root, name)
	}
	root[key] = block
}

// set writes value at a path relative to the tool block, so callers name
// "tunables.targetUsers" rather than "jmeter.tunables.targetUsers".
func (w *toolWriter) set(path string, value any) error {
	return api.SetPath(w.root, w.key+"."+path, value)
}

func (w *toolWriter) get(path string) (any, bool) {
	return api.GetPath(w.root, w.key+"."+path)
}

// list reads a path holding an array of maps, the shape both a config file and
// an earlier flag produce.
func (w *toolWriter) list(path string) []any {
	value, found := w.get(path)
	if !found {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return items
}

func joinTools(tools []api.ToolType) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = string(t)
	}
	return strings.Join(names, ", ")
}

// setMode records the authoring mode. K6's ui mode was retired with the
// unified schema; a test that used it is now a script.
func (w *toolWriter) setMode(mode api.Mode) error {
	switch mode {
	case api.ModeScript, api.ModeImage:
	case "ui":
		return fmt.Errorf("--mode ui is no longer supported: define the test as a script with --script, or as a container with --mode image")
	default:
		return fmt.Errorf("unsupported --mode %q: supported values are script, image", mode)
	}
	return w.set("mode", string(mode))
}

// currentMode reports the mode already recorded in the block, which is how a
// config file's mode survives when --mode is not passed.
func (w *toolWriter) currentMode() api.Mode {
	value, found := w.get("mode")
	if !found {
		return ""
	}
	text, _ := value.(string)
	return api.Mode(text)
}

// scalarKind is the natural type of a leaf, used to reject "--target-users
// twenty" while still accepting "--target-users <+input>".
type scalarKind int

const (
	kindString scalarKind = iota
	kindInt
	kindFloat
	kindBool
)

// isExpression spots "<+input>" or "<+variable.NAME>". Either may appear at a
// leaf of any type, so neither is parsed.
func isExpression(raw string) bool {
	return strings.HasPrefix(raw, "<+") && strings.HasSuffix(raw, ">")
}

// scalar gives a flag string its JSON type, leaving expressions verbatim.
func scalar(flag, raw string, kind scalarKind) (any, error) {
	if isExpression(raw) {
		return raw, nil
	}
	switch kind {
	case kindInt:
		number, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid --%s %q: expected a whole number, or %s to supply it at run time", flag, raw, api.RuntimeInput)
		}
		return number, nil
	case kindFloat:
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --%s %q: expected a number, or %s to supply it at run time", flag, raw, api.RuntimeInput)
		}
		return number, nil
	}
	return raw, nil
}

// tunableFlags maps each load-shape flag to the leaf it writes. An empty
// onlyFor means every engine accepts the flag.
var tunableFlags = []struct {
	flag    string
	path    string
	kind    scalarKind
	onlyFor api.ToolType
}{
	{flag: "target-url", path: "tunables.targetUrl", kind: kindString},
	{flag: "target-users", path: "tunables.targetUsers", kind: kindInt},
	{flag: "ramp-up", path: "tunables.rampUpTimeSec", kind: kindInt},
	{flag: "duration", path: "tunables.durationSeconds", kind: kindInt},
	{flag: "worker-count", path: "tunables.workerCount", kind: kindInt},
	{flag: "max-duration", path: "tunables.maxDurationSec", kind: kindInt},
	{flag: "spawn-rate", path: "tunables.spawnRate", kind: kindFloat, onlyFor: api.ToolLocust},
	{flag: "host-url", path: "tunables.hostUrl", kind: kindString, onlyFor: api.ToolK6},
	{flag: "iterations", path: "tunables.iterations", kind: kindInt, onlyFor: api.ToolK6},
	{flag: "rps-limit", path: "tunables.rpsLimit", kind: kindInt, onlyFor: api.ToolK6},
}

// applyTunables writes the load-shape flags actually typed, rejecting one
// aimed at an engine that has no such setting.
func (w *toolWriter) applyTunables(set map[string]bool, value func(string) string) error {
	for _, tunable := range tunableFlags {
		if !set[tunable.flag] {
			continue
		}
		if tunable.onlyFor != "" && tunable.onlyFor != w.toolType {
			return fmt.Errorf("--%s is a %s setting and does not apply to %s", tunable.flag, tunable.onlyFor, w.toolType)
		}
		parsed, err := scalar(tunable.flag, value(tunable.flag), tunable.kind)
		if err != nil {
			return err
		}
		if err := w.set(tunable.path, parsed); err != nil {
			return err
		}
	}
	return nil
}

// applyEnvVars merges KEY=VALUE pairs into envVars. secret marks the value as
// a reference resolved at dispatch rather than a literal.
func (w *toolWriter) applyEnvVars(pairs []string, secret bool) error {
	flag := "env"
	if secret {
		flag = "env-secret"
	}

	existing := w.list("envVars")
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return fmt.Errorf("invalid --%s %q: expected KEY=VALUE", flag, pair)
		}
		entry := map[string]any{"key": key, "value": value}
		if secret {
			entry["secret"] = true
		}
		existing = upsertByField(existing, "key", key, entry)
	}

	return w.set("envVars", existing)
}

// applyProperties merges JMeter runtime properties.
func (w *toolWriter) applyProperties(pairs []string) error {
	if w.toolType != api.ToolJMeter {
		return fmt.Errorf("--property is a JMeter setting and does not apply to %s", w.toolType)
	}

	existing := w.list("properties")
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return fmt.Errorf("invalid --property %q: expected KEY=VALUE", pair)
		}
		existing = upsertByField(existing, "key", key, map[string]any{
			"key":   key,
			"value": literal(value),
		})
	}

	return w.set("properties", existing)
}

// applyMetricTags merges k6 metric tags, which label every sample the run emits.
func (w *toolWriter) applyMetricTags(pairs []string) error {
	if w.toolType != api.ToolK6 {
		return fmt.Errorf("--metric-tag is a K6 setting and does not apply to %s", w.toolType)
	}

	tags := map[string]any{}
	if value, found := w.get("metricTags"); found {
		if stored, ok := value.(map[string]any); ok {
			tags = stored
		}
	}
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return fmt.Errorf("invalid --metric-tag %q: expected KEY=VALUE", pair)
		}
		tags[key] = value
	}

	return w.set("metricTags", tags)
}

// applyThresholds appends pass/fail criteria. They key on metric and stat
// together, so p95 and p99 rules on the same metric coexist.
func (w *toolWriter) applyThresholds(specs []string) error {
	if w.toolType == api.ToolLocust {
		return fmt.Errorf("--threshold is not supported for Locust")
	}

	existing := w.list("thresholds")
	for _, spec := range specs {
		threshold, key, err := parseThreshold(spec)
		if err != nil {
			return err
		}
		existing = upsertByKey(existing, thresholdKey, key, threshold)
	}

	return w.set("thresholds", existing)
}

func thresholdKey(entry map[string]any) string {
	metric, _ := entry["metric"].(string)
	stat, _ := entry["stat"].(string)
	return metric + "/" + stat
}

func parseThreshold(spec string) (map[string]any, string, error) {
	fields, err := parseShorthand("threshold", spec)
	if err != nil {
		return nil, "", err
	}

	threshold := map[string]any{}
	for name, value := range fields {
		switch name {
		case "metric":
			threshold["metric"] = value
		case "stat":
			threshold["stat"] = value
		case "operator", "op":
			threshold["operator"] = value
		case "value":
			threshold["value"] = literal(value)
		case "abortOnFail", "abort-on-fail":
			flag, err := strconv.ParseBool(value)
			if err != nil {
				return nil, "", fmt.Errorf("invalid --threshold %q: abortOnFail must be true or false", spec)
			}
			threshold["abortOnFail"] = flag
		default:
			return nil, "", fmt.Errorf("invalid --threshold %q: unknown field %q, expected metric, stat, operator, value or abortOnFail", spec, name)
		}
	}

	metric, _ := threshold["metric"].(string)
	if metric == "" {
		return nil, "", fmt.Errorf("invalid --threshold %q: metric is required", spec)
	}
	operator, _ := threshold["operator"].(string)
	if operator == "" {
		return nil, "", fmt.Errorf("invalid --threshold %q: operator is required, one of %s", spec, strings.Join(api.ThresholdOperators, " "))
	}
	if !contains(api.ThresholdOperators, operator) {
		return nil, "", fmt.Errorf("invalid --threshold %q: unsupported operator %q, expected one of %s", spec, operator, strings.Join(api.ThresholdOperators, " "))
	}
	if _, present := threshold["value"]; !present {
		return nil, "", fmt.Errorf("invalid --threshold %q: value is required", spec)
	}

	return threshold, thresholdKey(threshold), nil
}

// k6OptionFlags is a fixed table rather than a free-form map so a misspelt
// option is caught here instead of being ignored by the runner.
var k6OptionFlags = []struct {
	flag string
	path string
	kind scalarKind
}{
	{flag: "k6-user-agent", path: "options.userAgent", kind: kindString},
	{flag: "k6-dns-strategy", path: "options.dnsStrategy", kind: kindString},
	{flag: "k6-batch-concurrent", path: "options.batchConcurrent", kind: kindInt},
	{flag: "k6-batch-per-host", path: "options.batchPerHost", kind: kindInt},
	{flag: "k6-no-connection-reuse", path: "options.noConnectionReuse", kind: kindBool},
	{flag: "k6-insecure-skip-tls-verify", path: "options.insecureSkipTLSVerify", kind: kindBool},
	{flag: "k6-discard-response-bodies", path: "options.discardResponseBodies", kind: kindBool},
}

// applyK6Options writes the k6 options explicitly set, rejecting any aimed at
// another engine.
func (w *toolWriter) applyK6Options(set map[string]bool, text func(string) string, number func(string) int, flag func(string) bool) error {
	for _, option := range k6OptionFlags {
		if !set[option.flag] {
			continue
		}
		if w.toolType != api.ToolK6 {
			return fmt.Errorf("--%s is a K6 setting and does not apply to %s", option.flag, w.toolType)
		}

		var value any
		switch option.kind {
		case kindInt:
			value = number(option.flag)
		case kindBool:
			value = flag(option.flag)
		default:
			value = text(option.flag)
		}
		if err := w.set(option.path, value); err != nil {
			return err
		}
	}
	return nil
}

// applySetFlags is the escape hatch for a leaf with no dedicated flag.
func (w *toolWriter) applySetFlags(assignments []string) error {
	for _, assignment := range assignments {
		path, value, found := strings.Cut(assignment, "=")
		path = strings.TrimSpace(path)
		if !found || path == "" {
			return fmt.Errorf("invalid --set %q: expected PATH=VALUE, for example tunables.targetUsers=500", assignment)
		}
		if err := w.set(path, literal(value)); err != nil {
			return err
		}
	}
	return nil
}

// applyRuntimeInputs marks leaves as supplied at run time. Same as
// "--set PATH=<+input>", which needs shell quoting on every invocation.
func (w *toolWriter) applyRuntimeInputs(paths []string) error {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || strings.Contains(path, "=") {
			return fmt.Errorf("invalid --runtime-input %q: expected a path, for example tunables.targetUsers", path)
		}
		if err := w.set(path, api.RuntimeInput); err != nil {
			return err
		}
	}
	return nil
}

// literal gives a flag string its JSON type, leaving Harness expressions and
// anything non-numeric as text.
func literal(raw string) any {
	if isExpression(raw) {
		return raw
	}
	if number, err := strconv.Atoi(raw); err == nil {
		return number
	}
	if number, err := strconv.ParseFloat(raw, 64); err == nil {
		return number
	}
	if boolean, err := strconv.ParseBool(raw); err == nil {
		return boolean
	}
	return raw
}

// parseShorthand splits the "name=value,name=value" form the structured flags
// take, as the AWS CLI does. Values may not contain a comma.
func parseShorthand(flag, spec string) (map[string]string, error) {
	fields := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, found := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return nil, fmt.Errorf("invalid --%s %q: expected name=value pairs separated by commas", flag, spec)
		}
		fields[name] = strings.TrimSpace(value)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid --%s %q: expected name=value pairs separated by commas", flag, spec)
	}
	return fields, nil
}

// upsertByField replaces the entry whose field equals key, or appends, so
// repeating a flag overrides rather than duplicates.
func upsertByField(items []any, field, key string, entry map[string]any) []any {
	return upsertByKey(items, func(existing map[string]any) string {
		value, _ := existing[field].(string)
		return value
	}, key, entry)
}

func upsertByKey(items []any, keyOf func(map[string]any) string, key string, entry map[string]any) []any {
	for i, item := range items {
		existing, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if keyOf(existing) == key {
			items[i] = entry
			return items
		}
	}
	return append(items, entry)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// mergeVariables merges into the test's variables, which sit beside toolConfig
// rather than inside it and are referenced as <+variable.NAME>.
func mergeVariables(existing, incoming []api.Variable) []api.Variable {
	index := make(map[string]int, len(existing))
	for i, item := range existing {
		index[item.Name] = i
	}
	for _, item := range incoming {
		if i, found := index[item.Name]; found {
			existing[i].Value = item.Value
			continue
		}
		index[item.Name] = len(existing)
		existing = append(existing, item)
	}
	return existing
}

func applyIf(set map[string]bool, flag string, apply func()) {
	if set[flag] {
		apply()
	}
}
