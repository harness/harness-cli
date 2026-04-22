package maven

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/maven/command"

	"github.com/spf13/cobra"
)

func GetRootCmd(f *cmdutils.Factory) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "mvn [command] [args...]",
		Short: "mvn wrapper with firewall support",
		Long: `Wrap mvn commands with firewall integration.
Install and package commands get firewall support.
All other commands are passed through to native mvn.`,
	}

	rootCmd.AddCommand(command.NewMavenInstallCmd(f))
	rootCmd.AddCommand(command.NewMavenPackageCmd(f))

	// Passthrough: any unknown subcommand gets forwarded to native mvn
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runNativeMvn(args)
	}

	return rootCmd
}

func runNativeMvn(args []string) error {
	mvnPath, err := exec.LookPath("mvn")
	if err != nil {
		return fmt.Errorf("mvn not found in PATH: %w", err)
	}

	cmd := exec.Command(mvnPath, args...)
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
