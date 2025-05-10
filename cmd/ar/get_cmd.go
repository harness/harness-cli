package ar

import (
	"github.com/spf13/cobra"
)

func getGetCommand(cmds ...*cobra.Command) *cobra.Command {

	// Artifact command
	artifactCmd := &cobra.Command{
		Use:   "get [ar-ref]",
		Short: "Artifact management commands",
		Long:  `Commands to manage Harness Artifact Registry artifacts`,
	}

	for _, cmd := range cmds {
		artifactCmd.AddCommand(cmd)
	}

	return artifactCmd
}
