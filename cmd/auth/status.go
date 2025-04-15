package auth

import (
	"fmt"
	"github.com/spf13/cobra"
)

func getStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check authentication status",
		Long:  `Display current authentication status and details`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Here you would implement the actual status check logic
			// For example, reading the saved credentials from config file
			// and displaying their status

			fmt.Println("Checking authentication status...")
			fmt.Println("You are authenticated to Harness")
			return nil
		},
	}

	return cmd
}
