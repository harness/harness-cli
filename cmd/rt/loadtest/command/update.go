package command

import (
	"fmt"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"

	"github.com/spf13/cobra"
)

func NewUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <identity>",
		Short: "Update an existing load test",
		Long: `Change the definition of an existing load test.

Only the fields you pass are changed; everything else keeps its stored value.
Tunables are merged into the tool configuration already on the test, so
adjusting the user count leaves the script and the rest of the config alone.

The identity and the tool type of a test cannot be changed after creation.
Use --config to apply a whole definition from a file.`,
		Example: `  # Raise the load and hold it for longer
  hc rt loadtest update checkout-load --target-users 500 --duration 1800

  # Retarget at a different environment
  hc rt loadtest update checkout-load --environment-id production --infra-id prod-cluster

  # Keep run pods around for debugging
  hc rt loadtest update checkout-load --cleanup-policy retain`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			identity := args[0]

			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			// Tunable flags patch one field of a nested tool block, so fetch
			// and merge rather than replace wholesale.
			current, err := sess.client.Get(cmd.Context(), identity)
			if err != nil {
				return err
			}

			request, err := buildUpdateRequest(cmd, current)
			if err != nil {
				return err
			}

			updated, err := sess.client.Update(cmd.Context(), identity, *request)
			if err != nil {
				return err
			}

			return sess.printer.Print(updated, loadTestTable([]*api.LoadTest{updated}))
		},
	}

	flags := cmd.Flags()

	registerConfigFlag(cmd, "Path to a JSON definition of the changes; flags override its values")

	flags.String("name", "", "New display name")
	flags.String("description", "", "New description")
	flags.String("infra-id", "", "Move the test to a different infrastructure")
	flags.String("environment-id", "", "Move the test to a different environment")
	flags.String("target-type", "", "Infrastructure family: kubernetes or linux")
	flags.StringSlice("tag", nil, "Replace the tags with this set")
	flags.StringSlice("service-ref", nil, "Replace the linked chaos services with this set")
	flags.String("cleanup-policy", "", "Whether run resources are removed afterwards: delete or retain")

	registerToolConfigFlags(cmd)

	flags.StringArray("var", nil, "Set a variable as NAME=VALUE, referenced from config as <+variable.NAME>; repeat for several")

	flags.String("cpu-request", "", "CPU requested per pod, for example 500m")
	flags.String("cpu-limit", "", "CPU limit per pod, for example 2")
	flags.String("memory-request", "", "Memory requested per pod, for example 512Mi")
	flags.String("memory-limit", "", "Memory limit per pod, for example 2Gi")

	return cmd
}

func buildUpdateRequest(cmd *cobra.Command, current *api.LoadTest) (*api.UpdateRequest, error) {
	request := &api.UpdateRequest{}

	configPath, _ := cmd.Flags().GetString(FlagConfig)
	if configPath != "" {
		if err := LoadConfigFile(configPath, request); err != nil {
			return nil, err
		}
	}

	set := changedFlags(cmd)
	str := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }

	applyIf(set, "name", func() { request.Name = str("name") })
	applyIf(set, "description", func() { request.Description = str("description") })
	applyIf(set, "infra-id", func() { request.InfraIdentifier = str("infra-id") })
	applyIf(set, "environment-id", func() { request.EnvironmentIdentifier = str("environment-id") })
	applyIf(set, "target-type", func() { request.TargetType = str("target-type") })
	applyIf(set, "cleanup-policy", func() { request.CleanupPolicy = api.CleanupPolicy(str("cleanup-policy")) })
	applyIf(set, "tag", func() { request.Tags, _ = cmd.Flags().GetStringSlice("tag") })
	applyIf(set, "service-ref", func() { request.ServiceReferences, _ = cmd.Flags().GetStringSlice("service-ref") })

	if err := validateCleanupPolicy(request.CleanupPolicy); err != nil {
		return nil, err
	}

	if set["var"] {
		pairs, _ := cmd.Flags().GetStringArray("var")
		parsed, err := parseVariables(pairs)
		if err != nil {
			return nil, err
		}
		if request.Variables == nil {
			request.Variables = current.Variables
		}
		request.Variables = mergeVariables(request.Variables, parsed)
	}

	if touchesToolConfig(set) || request.ToolConfig != nil {
		// The stored config is the base. Sending only the changed leaf would
		// drop everything the flag did not mention, including the script.
		if request.ToolConfig == nil {
			request.ToolConfig = current.ToolConfig
		}
		if request.ToolConfig == nil {
			request.ToolConfig = map[string]any{}
		}
		writer, err := newToolWriter(request.ToolConfig, current.ToolType)
		if err != nil {
			return nil, err
		}
		if set["mode"] {
			if err := writer.setMode(api.Mode(str("mode"))); err != nil {
				return nil, err
			}
		}
		if err := applyToolConfigFlags(cmd, writer, set); err != nil {
			return nil, err
		}
	}

	if err := applyUpdateResourceFlags(cmd, request, current, set); err != nil {
		return nil, err
	}

	return request, nil
}

// toolConfigFlags are the flags whose values live inside toolConfig. With none
// set the block is omitted, so a rename does not rewrite the configuration.
var toolConfigFlags = []string{
	"mode", "target-url", "target-users", "ramp-up", "duration", "worker-count",
	"max-duration", "spawn-rate", "host-url", "iterations", "rps-limit",
	"image", "entrypoint", "image-pull-secret", "run-arg",
	"env", "env-secret", "property", "metric-tag", "threshold",
	"k6-user-agent", "k6-dns-strategy", "k6-batch-concurrent", "k6-batch-per-host",
	"k6-no-connection-reuse", "k6-insecure-skip-tls-verify", "k6-discard-response-bodies",
	"set", "runtime-input",
}

func touchesToolConfig(set map[string]bool) bool {
	for _, flag := range toolConfigFlags {
		if set[flag] {
			return true
		}
	}
	return false
}

// applyUpdateResourceFlags patches the stored resource block, since sending a
// partial one would drop the fields left out.
func applyUpdateResourceFlags(cmd *cobra.Command, request *api.UpdateRequest, current *api.LoadTest, set map[string]bool) error {
	if request.Resources == nil {
		request.Resources = current.Resources
	}

	create := &api.CreateRequest{Resources: request.Resources}
	if err := applyResourceFlags(cmd, create, set); err != nil {
		return err
	}
	request.Resources = create.Resources

	return nil
}

// NewSyncCmd pulls the newest template revision into a reference import.
func NewSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync <identity>",
		Short: "Pull the latest template revision into a load test",
		Long: `Update a load test that was imported from a template to the newest
revision of that template.

Only tests created with "hc rt loadtest create-from-template" as a reference
import can be synced. "hc rt loadtest get" reports whether an update is
available in the templateUpdateAvailable field.`,
		Example:      `  hc rt loadtest sync checkout-load`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := newSession(cmd)
			if err != nil {
				return err
			}

			updated, err := sess.client.SyncTemplate(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Synced %q to the latest template revision.\n", args[0])
			return sess.printer.Print(updated, loadTestTable([]*api.LoadTest{updated}))
		},
	}
}
