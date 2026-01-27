package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewListProjectCmd creates the list command for projects
func NewListProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  "Lists all projects in the specified organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("project list command not yet implemented")
		},
	}

	return cmd
}
