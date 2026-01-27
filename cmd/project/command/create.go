package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCreateProjectCmd creates the create command for projects
func NewCreateProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [project-id]",
		Short: "Create a new project",
		Long:  "Creates a new project in the specified organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("project create command not yet implemented")
		},
	}

	return cmd
}
