package command

import (
	"errors"

	"github.com/spf13/cobra"
	client "harness/internal/api/ar"
)

// newDeleteArtifactCmd wires up:
//
//	hns ar artifact delete <args>
func NewDeleteArtifactCmd() *cobra.Command {
	var host string
	var format string
	cmd := &cobra.Command{
		Use:   "artifact delete n ",
		Short: "Delete an artifact from a registry",
		Long:  "Deletes a specific artifact and all its versions from the Harness Artifact Registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create client
			_, err := client.NewClient(host, nil)
			if err != nil {
				return err
			}

			// Validate required arguments
			if len(args) < 1 {
				return errors.New("missing required arguments: n ")
			}

			// Call API
			//resp, err := cli.DeleteArtifact(context.Background(), args[0])
			//if err != nil {
			//	return err
			//}

			// Format output based on format flag
			//switch format {
			//case "json":
			// TODO: output JSON here
			//	fmt.Printf("%+v\n", resp)
			//case "table":
			// TODO: format as table
			//	fmt.Printf("%+v\n", resp)
			//default:
			//	fmt.Printf("%+v\n", resp)
			//}
			return nil
		},
	}

	// Common flags
	cmd.Flags().StringVar(&host, "host", "http://localhost:8080", "service base URL")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")

	// TODO: Add any command-specific flags here

	return cmd
}
