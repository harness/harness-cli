package command

import (
	"fmt"
	"math"
	"strconv"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// settableTable lists variables and runtime inputs together — a test may have
// only the latter — but keeps KIND, since --value and --runtime-value differ.
func settableTable(variables []api.Variable, inputs []api.Input) *Table {
	table := &Table{Headers: []string{"KIND", "NAME OR PATH", "VALUE", "TYPE", "REQUIRED", "DESCRIPTION"}}
	for _, item := range variables {
		table.Rows = append(table.Rows, []string{
			"variable",
			item.Name,
			fmt.Sprintf("%v", item.Value),
			item.Type,
			strconv.FormatBool(item.Required),
			item.Description,
		})
	}
	for _, item := range inputs {
		path := item.Path
		if path == "" {
			path = item.Name
		}
		table.Rows = append(table.Rows, []string{
			"input",
			path,
			formatDefault(item.Value),
			item.Type,
			strconv.FormatBool(item.Required),
			item.Description,
		})
	}
	return table
}

func variableTable(items []api.Variable) *Table {
	table := &Table{Headers: []string{"NAME", "VALUE", "TYPE", "REQUIRED", "DESCRIPTION"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.Name,
			fmt.Sprintf("%v", item.Value),
			item.Type,
			strconv.FormatBool(item.Required),
			item.Description,
		})
	}
	return table
}

func runTable(items []*api.Run) *Table {
	table := &Table{Headers: []string{"RUN", "LOAD TEST", "STATUS", "USERS", "DURATION", "STARTED", "FINISHED"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.Identity,
			item.LoadTestIdentity,
			string(item.Status),
			strconv.Itoa(item.TargetUsers),
			optionalSeconds(item.DurationSeconds),
			optionalString(item.StartedAt),
			optionalString(item.FinishedAt),
		})
	}
	return table
}

func revisionTable(items []*api.ScriptRevision) *Table {
	table := &Table{Headers: []string{"REVISION", "IDENTITY", "CREATED", "BY", "DESCRIPTION"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			strconv.Itoa(item.RevisionNumber),
			item.Identity,
			item.CreatedAt,
			item.CreatedBy,
			item.Description,
		})
	}
	return table
}

func templateTable(items []*api.Template) *Table {
	table := &Table{Headers: []string{"IDENTITY", "REVISION", "NAME", "TOOL", "HUB", "UPDATED"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.Identity,
			item.Revision,
			item.Name,
			string(item.ToolType),
			item.HubIdentity,
			item.UpdatedAt,
		})
	}
	return table
}

// templateRevisionTable views the same envelope as templateTable, leading with
// the revision rather than the identity, which is the same for every row.
func templateRevisionTable(items []*api.Template) *Table {
	table := &Table{Headers: []string{"REVISION", "NAME", "CREATED", "BY", "DESCRIPTION"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.Revision,
			item.Name,
			item.CreatedAt,
			item.CreatedBy,
			item.Description,
		})
	}
	return table
}

// inputTable lists the toolConfig leaves left as "<+input>". The path is the
// first column because it, not the name, is what --runtime-value takes.
func inputTable(items []api.Input) *Table {
	table := &Table{Headers: []string{"PATH", "TYPE", "REQUIRED", "DEFAULT", "DESCRIPTION"}}
	for _, item := range items {
		path := item.Path
		if path == "" {
			path = item.Name
		}
		table.Rows = append(table.Rows, []string{
			path,
			item.Type,
			strconv.FormatBool(item.Required),
			formatDefault(item.Default),
			item.Description,
		})
	}
	return table
}

func loadTestTable(items []*api.LoadTest) *Table {
	table := &Table{Headers: []string{"IDENTITY", "NAME", "TOOL", "INFRASTRUCTURE", "ENVIRONMENT", "USERS", "DURATION"}}
	for _, item := range items {
		table.Rows = append(table.Rows, []string{
			item.Identity,
			item.Name,
			string(item.ToolType),
			item.InfraIdentifier,
			item.EnvironmentIdentifier,
			tunable(item, "targetUsers", item.TargetUsers),
			tunable(item, "durationSeconds", item.DurationSeconds),
		})
	}
	return table
}

// tunable reads a setting from the tool's own tunables block. The top-level
// targetUsers and durationSeconds on a load test are display projections of
// exactly these values, kept for backward compatibility and due to be removed,
// so the tunables are the source of truth and the projection is only a
// fallback for as long as it is still sent.
func tunable(test *api.LoadTest, name, projected string) string {
	key := api.ToolKey(test.ToolType)
	if key == "" {
		return projected
	}

	tool, ok := test.ToolConfig[key].(map[string]any)
	if !ok {
		return projected
	}

	tunables, ok := tool["tunables"].(map[string]any)
	if !ok {
		return projected
	}

	value, ok := tunables[name]
	if !ok || value == nil {
		return projected
	}

	return formatNumber(value)
}

// formatNumber keeps a whole number whole. JSON decodes every number into a
// float64, and %v renders a large one in scientific notation, so a duration of
// 1000000 would print as 1e+06.
func formatNumber(value any) string {
	switch typed := value.(type) {
	case float64:
		if typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", value)
	}
}

func formatDefault(value any) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%v", value)
}

func optionalSeconds(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value) + "s"
}

func optionalString(value *string) string {
	if value == nil || *value == "" {
		return "-"
	}
	return *value
}
