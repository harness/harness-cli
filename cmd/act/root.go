package act

import (
	"github.com/spf13/cobra"
)

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "act [pipeline.yaml]",
		Short: "Run Harness CI pipelines locally via Docker",
		Long: `Run Harness CI pipelines locally using Docker containers.

Similar to nektos/act for GitHub Actions, this command parses a Harness CI
pipeline YAML and executes the steps locally in Docker containers.

Useful for validating pipeline logic, debugging step scripts, and testing
matrix strategies without pushing to CI.

Examples:
  hc act pipeline.yaml
  hc act pipeline.yaml --docker-host tcp://192.168.1.10:2375
  hc act pipeline.yaml --stage build --dry-run
  hc act pipeline.yaml --env FOO=bar --env BAZ=qux`,
		Args: cobra.ExactArgs(1),
		RunE: runAct,
	}

	rootCmd.Flags().String("docker-host", "", "Docker daemon socket (default: from DOCKER_HOST env or unix:///var/run/docker.sock)")
	rootCmd.Flags().Bool("dry-run", false, "Parse and validate the pipeline without executing")
	rootCmd.Flags().String("stage", "", "Run only the named stage (default: all stages)")
	rootCmd.Flags().StringSlice("env", nil, "Extra environment variables passed to all steps (KEY=VALUE)")
	rootCmd.Flags().String("network", "", "Docker network to attach containers to")
	rootCmd.Flags().Bool("no-pull", false, "Skip pulling images (use local images only)")
	rootCmd.Flags().String("default-image", "alpine:latest", "Default container image when none specified")

	return rootCmd
}
