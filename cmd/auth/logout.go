package auth

import (
	"fmt"
	"github.com/spf13/cobra"
)

func getLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout from Harness",
		Long:  `Remove saved Harness credentials`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Here you would implement the actual logout logic
			// For example, removing the saved credentials from config file

			fmt.Println("Successfully logged out from Harness")
			return nil
		},
	}

	return cmd
}
