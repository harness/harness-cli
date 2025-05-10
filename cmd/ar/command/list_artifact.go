package command

import (
	"github.com/spf13/cobra"
	client "harness/internal/api/ar"
)

// newListArtifactCmd wires up:
//
//	hns ar artifact list
func NewListArtifactCmd() *cobra.Command {
	var host string
	var format string
	cmd := &cobra.Command{
		Use:   "artifact list",
		Short: "List all artifacts in a registry",
		Long:  "Lists all artifacts available in a specific Harness Artifact Registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create client
			_, err := client.NewClient(host, nil)
			if err != nil {
				return err
			}

			// Call API
			//resp, err := cli.GetAllArtifactsByRegistry(context.Background())
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
