package ar //  ←  or ci, cdn, …

import "github.com/spf13/cobra"

// ServiceCmd is the command all generated sub-commands attach to.
var ServiceCmd = &cobra.Command{
	Use:   "ar",
	Short: "Artifact Registry service commands",
}

func NewCommand() *cobra.Command {
	return ServiceCmd
}
