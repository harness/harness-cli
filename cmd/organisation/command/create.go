package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCreateOrganisationCmd creates the create command for organisations
func NewCreateOrganisationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [org-id]",
		Short: "Create a new organisation",
		Long:  "Creates a new organisation in the account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("organisation create command not yet implemented")
		},
	}

	return cmd
}
