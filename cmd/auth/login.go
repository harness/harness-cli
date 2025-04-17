package auth

import (
	"fmt"
	"github.com/spf13/cobra"
)

func getLoginCmd() *cobra.Command {
	var (
		apiURL    string
		token     string
		accountID string
		orgID     string
		projectID string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Harness",
		Long:  `Authenticate with Harness services and save credentials for future use`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Here you would implement the actual login logic
			// For example, validating the token with Harness API
			// and saving the credentials to a config file

			fmt.Println("Successfully logged into Harness")
			return nil
		},
	}

	// Add flags specific to login
	cmd.Flags().StringVar(&apiURL, "clients-url", "", "Harness API URL")
	cmd.Flags().StringVar(&token, "token", "", "Authentication token")
	cmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	cmd.Flags().StringVar(&orgID, "org", "", "Organization ID")
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID")

	// Mark required flags
	cmd.MarkFlagRequired("clients-url")
	cmd.MarkFlagRequired("token")
	cmd.MarkFlagRequired("account")

	return cmd
}
