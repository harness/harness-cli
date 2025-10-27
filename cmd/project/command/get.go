package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewGetProjectCmd creates the get command for projects
func NewGetProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [project-id]",
		Short: "Get project details",
		Long:  "Retrieves detailed information about a specific project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("project get command not yet implemented")
		},
	}

	return cmd
}
