package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewListOrganisationCmd creates the list command for organisations
func NewListOrganisationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all organisations",
		Long:  "Lists all organisations in the account",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("organisation list command not yet implemented")
		},
	}

	return cmd
}
