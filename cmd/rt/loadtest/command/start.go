package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

// NewStartCmd is "start", not "run": "run" is the sub-group for run records,
// and cobra resolves a subcommand before an argument.
func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <identity>",
		Short: "Start a run of a load test",
		Long: `Start a run of an existing load test.

The run inherits everything from the load test definition. Use --value to
override a variable by name, and --runtime-value to supply a configuration
field that was authored as the "<+input>" sentinel. Neither changes the stored
definition.

A runtime input is addressed by path. List the paths a test accepts with
"hc rt loadtest variables <identity>" and pass one through as printed, or
abbreviate it to any trailing part that is still unambiguous — for a k6 test
".toolConfig.k6.tunables.targetUsers", "tunables.targetUsers" and
"targetUsers" all name the same field. A path that matches nothing is rejected
before the run starts, listing the paths the test does accept.

The command returns as soon as the run is accepted. Add --watch to follow it
until it finishes, which also makes the exit status reflect the outcome.`,
		Example: `  # Start a run and return immediately
  hc rt loadtest start checkout-load

  # Start a run with a name, and follow it to completion
  hc rt loadtest start checkout-load --run-name "release-2.4 baseline" --watch

  # Override a variable and a runtime input for this run only
  hc rt loadtest start checkout-load --value tier=premium \
    --runtime-value .toolConfig.k6.tunables.targetUsers=500

  # A trailing part of the path names the same input
  hc rt loadtest start checkout-load --runtime-value tunables.targetUsers=500`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity := args[0]

			request, err := buildRunRequest(cmd)
			if err != nil {
				return err
			}

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			if err := resolveRuntimeValues(cmd, sess, identity, request); err != nil {
				return err
			}

			started, err := sess.client.CreateRun(cmd.Context(), identity, *request)
			if err != nil {
				return err
			}

			watch, _ := cmd.Flags().GetBool("watch")
			if !watch {
				fmt.Fprintf(cmd.ErrOrStderr(), "Started run %s. Follow it with \"hc rt loadtest run watch %s\".\n",
					started.Identity, started.Identity)
				return sess.printer.Print(started, runTable([]*api.Run{started}))
			}

			return watchRun(cmd, sess, started.Identity)
		},
	}

	flags := cmd.Flags()
	flags.String("run-name", "", "Name for this run, shown in listings")
	flags.StringArray("value", nil, "Override a variable as NAME=VALUE; repeat for several")
	flags.StringArray("runtime-value", nil, "Supply a runtime input as PATH=VALUE, for example .toolConfig.tunables.targetUsers=500; see \"hc rt loadtest variables\"")
	flags.Bool("watch", false, "Follow the run until it reaches a terminal state")
	flags.Duration("interval", defaultPollInterval, "How often to poll while watching")
	flags.Duration("watch-timeout", 0, "Give up watching after this long (0 waits indefinitely)")

	return cmd
}

func buildRunRequest(cmd *cobra.Command) (*api.CreateRunRequest, error) {
	name, _ := cmd.Flags().GetString("run-name")

	// Identity names the run, not the load test; left empty CreateRun generates
	// a unique one.
	request := &api.CreateRunRequest{Name: name}

	values, _ := cmd.Flags().GetStringArray("value")
	for _, pair := range values {
		key, value, found := strings.Cut(pair, "=")
		if !found || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --value %q: expected NAME=VALUE", pair)
		}
		request.Values = append(request.Values, api.RunValue{Name: strings.TrimSpace(key), Value: value})
	}

	runtimeValues, _ := cmd.Flags().GetStringArray("runtime-value")
	for _, pair := range runtimeValues {
		path, value, found := strings.Cut(pair, "=")
		if !found || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("invalid --runtime-value %q: expected PATH=VALUE", pair)
		}
		if request.RuntimeValues == nil {
			request.RuntimeValues = map[string]any{}
		}
		request.RuntimeValues[strings.TrimSpace(path)] = typedValue(value)
	}

	return request, nil
}

// resolveRuntimeValues expands a path into the full form the service keys
// inputs under, by asking the test what it accepts and matching on the tail.
func resolveRuntimeValues(cmd *cobra.Command, sess *session, identity string, request *api.CreateRunRequest) error {
	if len(request.RuntimeValues) == 0 {
		return nil
	}

	variables, err := sess.client.GetVariables(cmd.Context(), identity)
	if err != nil {
		return err
	}

	settable := make([]string, 0, len(variables.Inputs))
	for _, input := range variables.Inputs {
		settable = append(settable, input.Path)
	}

	resolved := make(map[string]any, len(request.RuntimeValues))
	for path, value := range request.RuntimeValues {
		match, err := matchRuntimePath(identity, path, settable)
		if err != nil {
			return err
		}
		resolved[match] = value
	}

	request.RuntimeValues = resolved
	return nil
}

func matchRuntimePath(identity, path string, settable []string) (string, error) {
	bare := strings.TrimLeft(path, ".")

	var matched []string
	for _, candidate := range settable {
		if candidate == "."+bare || strings.HasSuffix(candidate, "."+bare) {
			matched = append(matched, candidate)
		}
	}

	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		if len(settable) == 0 {
			return "", fmt.Errorf("load test %q has no runtime inputs, so %s cannot be supplied for a run: author the field as \"<+input>\" in the definition, or override a variable with --value NAME=VALUE",
				identity, path)
		}
		return "", fmt.Errorf("load test %q has no runtime input %s; it accepts %s",
			identity, path, strings.Join(settable, ", "))
	default:
		return "", fmt.Errorf("%s names more than one runtime input of load test %q: %s; give one of them in full",
			path, identity, strings.Join(matched, ", "))
	}
}

// typedValue keeps numbers and booleans unquoted; an input bound to an integer
// field rejects a string.
func typedValue(raw string) any {
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
