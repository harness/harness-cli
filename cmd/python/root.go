package python

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/python/command"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "pip [command] [args...]",
		Short: "pip wrapper with firewall support",
		Long: `Wrap pip commands with firewall integration.
Install command gets firewall support.
All other commands are passed through to native pip.`,
	}

	rootCmd.AddCommand(command.NewPipInstallCmd(f))

	// Passthrough: any unknown subcommand gets forwarded to native pip
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runNativePip(args)
	}

	return rootCmd
}

func runNativePip(args []string) error {
	pipPath, err := exec.LookPath("pip")
	if err != nil {
		pipPath, err = exec.LookPath("pip3")
		if err != nil {
			return fmt.Errorf("pip/pip3 not found in PATH: %w", err)
		}
	}

	cmd := exec.Command(pipPath, args...)
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
