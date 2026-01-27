package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewGetOrganisationCmd creates the get command for organisations
func NewGetOrganisationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [org-id]",
		Short: "Get organisation details",
		Long:  "Retrieves detailed information about a specific organisation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("organisation get command not yet implemented")
		},
	}

	return cmd
}
