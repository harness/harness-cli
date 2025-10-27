package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewDeleteProjectCmd creates the delete command for projects
func NewDeleteProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [project-id]",
		Short: "Delete a project",
		Long:  "Deletes a project from the specified organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("project delete command not yet implemented")
		},
	}

	return cmd
}
