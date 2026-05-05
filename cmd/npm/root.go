package npm

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/npm/command"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "npm [command] [args...]",
		Short: "npm wrapper with build-info and firewall support",
		Long: `Wrap npm commands with build-info collection and firewall integration.
Install and ci commands get build-info + firewall support.
All other commands are passed through to native npm.`,
	}

	// Register wrapped commands as proper subcommands
	rootCmd.AddCommand(command.NewNpmInstallCmd(f))
	rootCmd.AddCommand(command.NewNpmCiCmd(f))

	// Passthrough: any unknown subcommand gets forwarded to native npm
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runNativeNpm(args)
	}

	return rootCmd
}

// runNativeNpm passes all args directly to the native npm binary.
func runNativeNpm(args []string) error {
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found in PATH: %w", err)
	}

	cmd := exec.Command(npmPath, args...)
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
