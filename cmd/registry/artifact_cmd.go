package registry

import (
	"fmt"
	"harness/internal/api/ar"
	"harness/internal/config"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	artifactLabelsFlag []string
	artifactFlag       string
	versionFlag        string
)

func getArtifactsCmd() *cobra.Command {

	// Artifact command
	artifactCmd := &cobra.Command{
		Use:   "artifact",
		Short: "Artifact management commands",
		Long:  `Commands to manage Harness Artifact Registry artifacts`,
	}
	// List artifacts command
	listArtifactsCmd := &cobra.Command{
		Use:   "list [registry-ref]",
		Short: "List artifacts in a registry",
		Args:  cobra.ExactArgs(1),
		Run:   listArtifacts,
	}
	listArtifactsCmd.Flags().IntVar(&pageFlag, "page", 0, "Page number")
	listArtifactsCmd.Flags().IntVar(&sizeFlag, "size", 20, "Page size")
	listArtifactsCmd.Flags().StringVar(&searchTermFlag, "search", "", "Search term")

	// Get artifact details command
	getArtifactCmd := &cobra.Command{
		Use:   "get [registry-ref] [artifact] [version]",
		Short: "Get artifact version details",
		Args:  cobra.ExactArgs(3),
		Run:   getArtifact,
	}

	// Delete artifact version command
	deleteArtifactCmd := &cobra.Command{
		Use:   "delete [registry-ref] [artifact] [version]",
		Short: "Delete artifact version",
		Args:  cobra.ExactArgs(3),
		Run:   deleteArtifact,
	}

	// Add commands to artifact command
	artifactCmd.AddCommand(listArtifactsCmd)
	artifactCmd.AddCommand(getArtifactCmd)
	artifactCmd.AddCommand(deleteArtifactCmd)

	return artifactCmd
}

func listArtifacts(cmd *cobra.Command, args []string) {
	registryRef := args[0]

	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Make API call
	resp, err := client.ListArtifacts(registryRef, artifactLabelsFlag, pageFlag, sizeFlag, searchTermFlag)
	if err != nil {
		fmt.Printf("Error listing artifacts: %v\n", err)
		os.Exit(1)
	}

	// Print response
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPACKAGE TYPE\tDOWNLOADS")
	for _, art := range resp.Data.Artifacts {
		fmt.Fprintf(w, "%s\t%s\t%d\n",
			art.Name,
			art.PackageType,
			art.DownloadsCount)
	}
	w.Flush()

	fmt.Printf("\nPage %d of %d (Total: %d artifacts)\n",
		resp.Data.PageIndex, resp.Data.PageCount, resp.Data.ItemCount)
}

func getArtifact(cmd *cobra.Command, args []string) {
	registryRef := args[0]
	artifact := args[1]
	version := args[2]

	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Make API call
	resp, err := client.GetArtifactDetail(registryRef, artifact, version)
	if err != nil {
		fmt.Printf("Error getting artifact details: %v\n", err)
		os.Exit(1)
	}

	// Print response
	fmt.Println("Artifact details:")
	fmt.Printf("  Name: %s\n", resp.Data.Name)
	fmt.Printf("  Version: %s\n", resp.Data.Version)
	if resp.Data.Size != "" {
		fmt.Printf("  Size: %s\n", resp.Data.Size)
	}
	if resp.Data.CreatedAt != "" {
		fmt.Printf("  Created At: %s\n", resp.Data.CreatedAt)
	}
	if resp.Data.ModifiedAt != "" {
		fmt.Printf("  Modified At: %s\n", resp.Data.ModifiedAt)
	}
	if len(resp.Data.Labels) > 0 {
		fmt.Printf("  Labels: %s\n", strings.Join(resp.Data.Labels, ", "))
	}
	if resp.Data.DownloadURL != "" {
		fmt.Printf("  Download URL: %s\n", resp.Data.DownloadURL)
	}
}

func deleteArtifact(cmd *cobra.Command, args []string) {
	registryRef := args[0]
	artifact := args[1]
	version := args[2]

	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Make API call
	err := client.DeleteArtifactVersion(registryRef, artifact, version)
	if err != nil {
		fmt.Printf("Error deleting artifact version: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Artifact '%s' version '%s' deleted successfully\n", artifact, version)
}
