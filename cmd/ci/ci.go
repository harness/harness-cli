package ci

import "github.com/spf13/cobra"

// ServiceCmd is the command all generated sub-commands attach to.
var ServiceCmd = &cobra.Command{
	Use:   "ci",
	Short: "CI service commands",
}

func NewCommand() *cobra.Command {
	return ServiceCmd
}
