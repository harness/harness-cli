package ar

import (
	"fmt"
	"harness/clients/ar"
	"harness/config"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	registryTypeFlag     string
	packageTypeFlag      string
	descriptionFlag      string
	labelsFlag           []string
	allowedPatternFlag   []string
	blockedPatternFlag   []string
	pageFlag             int
	sizeFlag             int
	searchTermFlag       string
	recursiveFlag        bool
	packageTypesListFlag []string
)

func getRegistryCmds() *cobra.Command {
	// Registry command
	registryCmd := &cobra.Command{
		Use:   "registry",
		Short: "Registry management commands",
		Long:  `Commands to manage Harness Artifact Registry registries`,
	}

	// Get ar command
	getRegistryCmd := &cobra.Command{
		Use:   "get [ar-ref]",
		Short: "Get ar details",
		Args:  cobra.ExactArgs(1),
		Run:   getRegistry,
	}

	// Delete ar command
	deleteRegistryCmd := &cobra.Command{
		Use:   "delete [ar-ref]",
		Short: "Delete ar",
		Args:  cobra.ExactArgs(1),
		Run:   deleteRegistry,
	}

	// List registries command
	listRegistriesCmd := &cobra.Command{
		Use:   "list",
		Short: "List registries",
		Run:   listRegistries,
	}
	listRegistriesCmd.Flags().IntVar(&pageFlag, "page", 0, "Page number")
	listRegistriesCmd.Flags().IntVar(&sizeFlag, "size", 20, "Page size")
	listRegistriesCmd.Flags().StringVar(&searchTermFlag, "search", "", "Search term")
	listRegistriesCmd.Flags().StringVar(&registryTypeFlag, "type", "", "Registry type (VIRTUAL, UPSTREAM)")
	listRegistriesCmd.Flags().StringSliceVar(&packageTypesListFlag, "package-types", nil,
		"Package types to filter (comma-separated)")
	listRegistriesCmd.Flags().BoolVar(&recursiveFlag, "recursive", false, "List registries recursively")

	// Add commands to ar command
	registryCmd.AddCommand(getRegistryCmd)
	registryCmd.AddCommand(deleteRegistryCmd)
	registryCmd.AddCommand(listRegistriesCmd)

	return registryCmd
}

func createRegistry(cmd *cobra.Command, args []string) {
	identifier := args[0]

	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Prepare request
	req := ar.RegistryRequest{
		Identifier:     identifier,
		PackageType:    packageTypeFlag,
		Description:    descriptionFlag,
		Labels:         labelsFlag,
		AllowedPattern: allowedPatternFlag,
		BlockedPattern: blockedPatternFlag,
	}

	// Make API call
	resp, err := client.CreateRegistry(req)
	if err != nil {
		fmt.Printf("Error creating ar: %v\n", err)
		os.Exit(1)
	}

	// Print response
	fmt.Println("Registry created successfully:")
	fmt.Printf("  Identifier: %s\n", resp.Data.Identifier)
	fmt.Printf("  Package Type: %s\n", resp.Data.PackageType)
	if resp.Data.Description != "" {
		fmt.Printf("  Description: %s\n", resp.Data.Description)
	}
	fmt.Printf("  URL: %s\n", resp.Data.URL)
	if len(resp.Data.Labels) > 0 {
		fmt.Printf("  Labels: %s\n", strings.Join(resp.Data.Labels, ", "))
	}
}

func getRegistry(cmd *cobra.Command, args []string) {
	registryRef := args[0]

	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Make API call
	resp, err := client.GetRegistry(registryRef)
	if err != nil {
		fmt.Printf("Error getting ar details: %v\n", err)
		os.Exit(1)
	}

	// Print response
	fmt.Println("Registry details:")
	fmt.Printf("  Identifier: %s\n", resp.Data.Identifier)
	fmt.Printf("  Package Type: %s\n", resp.Data.PackageType)
	if resp.Data.Description != "" {
		fmt.Printf("  Description: %s\n", resp.Data.Description)
	}
	fmt.Printf("  URL: %s\n", resp.Data.URL)
	if len(resp.Data.Labels) > 0 {
		fmt.Printf("  Labels: %s\n", strings.Join(resp.Data.Labels, ", "))
	}
	fmt.Printf("  Created At: %s\n", resp.Data.CreatedAt)
	fmt.Printf("  Modified At: %s\n", resp.Data.ModifiedAt)
}

func deleteRegistry(cmd *cobra.Command, args []string) {
	registryRef := args[0]

	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Make API call
	err := client.DeleteRegistry(registryRef)
	if err != nil {
		fmt.Printf("Error deleting ar: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Registry '%s' deleted successfully\n", registryRef)
}

func listRegistries(cmd *cobra.Command, args []string) {
	// Create client
	client := ar.NewHARClient(config.Global.APIBaseURL, config.Global.AuthToken, config.Global.AccountID,
		config.Global.OrgID, config.Global.ProjectID)

	// Make API call
	resp, err := client.ListRegistries(packageTypesListFlag, registryTypeFlag, pageFlag, sizeFlag, searchTermFlag,
		recursiveFlag)
	if err != nil {
		fmt.Printf("Error listing registries: %v\n", err)
		os.Exit(1)
	}

	// Print response
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "IDENTIFIER\tPACKAGE TYPE\tTYPE\tDESCRIPTION\tARTIFACTS\tDOWNLOADS\tSIZE")
	for _, reg := range resp.Data.Registries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
			reg.Identifier,
			reg.PackageType,
			reg.Type,
			reg.Description,
			reg.ArtifactsCount,
			reg.DownloadsCount,
			reg.RegistrySize)
	}
	w.Flush()

	fmt.Printf("\nPage %d of %d (Total: %d registries)\n",
		resp.Data.PageIndex, resp.Data.PageCount, resp.Data.ItemCount)
}
