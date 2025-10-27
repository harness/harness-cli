package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewDeleteOrganisationCmd creates the delete command for organisations
func NewDeleteOrganisationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [org-id]",
		Short: "Delete an organisation",
		Long:  "Deletes an organisation from the account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("organisation delete command not yet implemented")
		},
	}

	return cmd
}
