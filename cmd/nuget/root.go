package nuget

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/nuget/command"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "dotnet [command] [args...]",
		Short: "dotnet wrapper with firewall support",
		Long: `Wrap dotnet commands with firewall integration.
Restore and build commands get firewall support.
All other commands are passed through to native dotnet.`,
	}

	rootCmd.AddCommand(command.NewDotnetRestoreCmd(f))
	rootCmd.AddCommand(command.NewDotnetBuildCmd(f))

	// Passthrough: any unknown subcommand gets forwarded to native dotnet
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runNativeDotnet(args)
	}

	return rootCmd
}

func runNativeDotnet(args []string) error {
	dotnetPath, err := exec.LookPath("dotnet")
	if err != nil {
		return fmt.Errorf("dotnet not found in PATH: %w", err)
	}

	cmd := exec.Command(dotnetPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}
