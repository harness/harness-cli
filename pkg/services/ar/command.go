package ar

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-resty/resty/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// Config contains client configuration
type Config struct {
	BaseURL   string
	AuthToken string
	AccountID string
	SpaceID   string
}

// Default API configuration
var defaultConfig = Config{
	BaseURL: "https://app.harness.io/api/v1",
}

// NewARCommand creates the root AR command
func NewARCommand() *cobra.Command {
	config := defaultConfig

	cmd := &cobra.Command{
		Use:   "ar",
		Short: "Artifact Registry service commands",
		Long:  `Commands to interact with Harness Artifact Registry service`,
	}

	// Add global flags for AR service
	cmd.PersistentFlags().StringVar(&config.BaseURL, "api-url", defaultConfig.BaseURL, "Harness API base URL")
	cmd.PersistentFlags().StringVar(&config.AuthToken, "token", "", "Authentication token")
	cmd.PersistentFlags().StringVar(&config.AccountID, "account-id", "", "Harness account ID")
	cmd.PersistentFlags().StringVar(&config.SpaceID, "space-id", "", "Harness space ID")

	// Add subcommands for main resources
	cmd.AddCommand(newRegistryCmd(config))
	cmd.AddCommand(newArtifactCmd(config))

	return cmd
}

// RegistryDetails contains registry information
type RegistryDetails struct {
	Identifier    string   `json:"identifier"`
	Description   string   `json:"description,omitempty"`
	Type          string   `json:"type"`
	PackageType   string   `json:"packageType"`
	URL           string   `json:"url"`
	ArtifactCount int      `json:"artifactCount"`
	Labels        []string `json:"labels,omitempty"`
}

// ArtifactDetails contains artifact information
type ArtifactDetails struct {
	Name          string   `json:"name"`
	LatestVersion string   `json:"latestVersion"`
	RegistryPath  string   `json:"registryPath"`
	PackageType   string   `json:"packageType"`
	Labels        []string `json:"labels,omitempty"`
	DownloadCount int      `json:"downloadCount"`
}

// Registry group commands

// newRegistryCmd creates the registry command group
func newRegistryCmd(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage artifact registries",
		Long:  `Commands to manage Harness Artifact Registries`,
	}

	// Add registry subcommands
	cmd.AddCommand(newListRegistriesCmd(config))
	cmd.AddCommand(newGetRegistryCmd(config))
	cmd.AddCommand(newCreateRegistryCmd(config))
	cmd.AddCommand(newDeleteRegistryCmd(config))

	return cmd
}

// newListRegistriesCmd creates the registry list command
func newListRegistriesCmd(config APIConfig) *cobra.Command {
	var packageTypes []string
	var registryType string
	var pageNumber, pageSize int
	var searchTerm string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifact registries",
		Long:  `List all artifact registries in the account or space`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			
			// In a real implementation, we would use the generated client to call the API
			// For demonstration, we'll use mock data
			registries := []RegistryDetails{
				{
					Identifier:    "docker-hub-proxy",
					Description:   "Docker Hub Proxy Registry",
					Type:          "UPSTREAM",
					PackageType:   "DOCKER",
					URL:           "harness.io/docker-hub-proxy",
					ArtifactCount: 125,
					Labels:        []string{"proxy", "docker"},
				},
				{
					Identifier:    "internal-docker",
					Description:   "Internal Docker Registry",
					Type:          "VIRTUAL",
					PackageType:   "DOCKER",
					URL:           "harness.io/internal-docker",
					ArtifactCount: 75,
					Labels:        []string{"internal", "docker"},
				},
			}
			
			return outputRegistries(registries, format)
		},
	}

	cmd.Flags().StringSliceVar(&packageTypes, "package-type", nil, "Package type filter (DOCKER, MAVEN, etc.)")
	cmd.Flags().StringVar(&registryType, "registry-type", "", "Registry type (VIRTUAL, UPSTREAM)")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number for pagination")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size for pagination")
	cmd.Flags().StringVar(&searchTerm, "search", "", "Search term to filter registries")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "Whether to list registries recursively")

	return cmd
}

// newGetRegistryCmd creates command to get registry details
func newGetRegistryCmd(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [registry-identifier]",
		Short: "Get registry details",
		Long:  `Get details of a specific registry`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			regID := args[0]
			format, _ := cmd.Flags().GetString("format")
			
			// Mock registry data for demonstration
			registry := RegistryDetails{
				Identifier:    regID,
				Description:   "Example Registry",
				Type:          "DOCKER",
				PackageType:   "DOCKER",
				URL:           fmt.Sprintf("harness.io/%s", regID),
				ArtifactCount: 42,
				Labels:        []string{"example", "demo"},
			}
			
			return outputRegistries([]RegistryDetails{registry}, format)
		},
	}

	return cmd
}

// newCreateRegistryCmd creates command to create a registry
func newCreateRegistryCmd(config APIConfig) *cobra.Command {
	var identifier, description, packageType, registryType string
	var labels []string
	
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create artifact registry",
		Long:  `Create a new artifact registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			
			// Mock create registry
			registry := RegistryDetails{
				Identifier:    identifier,
				Description:   description,
				Type:          registryType,
				PackageType:   packageType,
				URL:           fmt.Sprintf("harness.io/%s", identifier),
				ArtifactCount: 0,
				Labels:        labels,
			}
			
			fmt.Println("Registry created successfully!")
			return outputRegistries([]RegistryDetails{registry}, format)
		},
	}
	
	cmd.Flags().StringVar(&identifier, "identifier", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&description, "description", "", "Registry description")
	cmd.Flags().StringVar(&packageType, "package-type", "", "Package type (DOCKER, MAVEN, etc.) (required)")
	cmd.Flags().StringVar(&registryType, "registry-type", "", "Registry type (VIRTUAL, UPSTREAM) (required)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Registry labels")
	
	cmd.MarkFlagRequired("identifier")
	cmd.MarkFlagRequired("package-type")
	cmd.MarkFlagRequired("registry-type")
	
	return cmd
}

// newDeleteRegistryCmd creates command to delete a registry
func newDeleteRegistryCmd(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [registry-identifier]",
		Short: "Delete artifact registry",
		Long:  `Delete an artifact registry by its identifier`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			regID := args[0]
			
			// In a real implementation, we would call the API
			fmt.Printf("Registry '%s' deleted successfully!\n", regID)
			
			return nil
		},
	}

	return cmd
}

// Artifact group commands

// newArtifactCmd creates the artifact command group
func newArtifactCmd(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage artifacts",
		Long:  `Commands to manage artifacts in registries`,
	}

	// Add artifact subcommands
	cmd.AddCommand(newListArtifactsCmd(config))
	cmd.AddCommand(newGetArtifactCmd(config))

	return cmd
}

// newListArtifactsCmd creates command to list artifacts
func newListArtifactsCmd(config APIConfig) *cobra.Command {
	var registryID string
	var pageNumber, pageSize int
	var searchTerm string
	var latestVersion, deployedArtifact bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts",
		Long:  `List artifacts in a registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			
			// Mock artifacts for demonstration
			artifacts := []ArtifactDetails{
				{
					Name:          "nginx",
					LatestVersion: "1.21.6",
					RegistryPath:  fmt.Sprintf("%s/nginx", registryID),
					PackageType:   "DOCKER",
					Labels:        []string{"web", "server"},
					DownloadCount: 1250,
				},
				{
					Name:          "redis",
					LatestVersion: "6.2.6",
					RegistryPath:  fmt.Sprintf("%s/redis", registryID),
					PackageType:   "DOCKER",
					Labels:        []string{"database", "cache"},
					DownloadCount: 975,
				},
			}
			
			return outputArtifacts(artifacts, format)
		},
	}

	cmd.Flags().StringVar(&registryID, "registry", "", "Registry identifier (required)")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number for pagination")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size for pagination")
	cmd.Flags().StringVar(&searchTerm, "search", "", "Search term to filter artifacts")
	cmd.Flags().BoolVar(&latestVersion, "latest", false, "Show only latest versions")
	cmd.Flags().BoolVar(&deployedArtifact, "deployed", false, "Show only deployed artifacts")
	
	cmd.MarkFlagRequired("registry")

	return cmd
}

// newGetArtifactCmd creates command to get artifact details
func newGetArtifactCmd(config APIConfig) *cobra.Command {
	var registryID string

	cmd := &cobra.Command{
		Use:   "get [artifact-name]",
		Short: "Get artifact details",
		Long:  `Get details of a specific artifact`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactName := args[0]
			format, _ := cmd.Flags().GetString("format")
			
			// Mock artifact data for demonstration
			artifact := ArtifactDetails{
				Name:          artifactName,
				LatestVersion: "1.0.0",
				RegistryPath:  fmt.Sprintf("%s/%s", registryID, artifactName),
				PackageType:   "DOCKER",
				Labels:        []string{"example", "demo"},
				DownloadCount: 350,
			}
			
			return outputArtifacts([]ArtifactDetails{artifact}, format)
		},
	}

	cmd.Flags().StringVar(&registryID, "registry", "", "Registry identifier (required)")
	cmd.MarkFlagRequired("registry")

	return cmd
}

// Output formatting helpers

// outputRegistries outputs registry information in the specified format
func outputRegistries(registries []RegistryDetails, format string) error {
	switch format {
	case "json":
		return outputJSON(registries)
	default: // table format
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"IDENTIFIER", "TYPE", "PACKAGE TYPE", "URL", "ARTIFACTS", "LABELS"})
		
		for _, r := range registries {
			table.Append([]string{
				r.Identifier,
				r.Type,
				r.PackageType,
				r.URL,
				fmt.Sprintf("%d", r.ArtifactCount),
				fmt.Sprintf("%v", r.Labels),
			})
		}
		
		table.Render()
		return nil
	}
}

// outputArtifacts outputs artifact information in the specified format
func outputArtifacts(artifacts []ArtifactDetails, format string) error {
	switch format {
	case "json":
		return outputJSON(artifacts)
	default: // table format
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"NAME", "LATEST VERSION", "REGISTRY PATH", "PACKAGE TYPE", "DOWNLOADS", "LABELS"})
		
		for _, a := range artifacts {
			table.Append([]string{
				a.Name,
				a.LatestVersion,
				a.RegistryPath,
				a.PackageType,
				fmt.Sprintf("%d", a.DownloadCount),
				fmt.Sprintf("%v", a.Labels),
			})
		}
		
		table.Render()
		return nil
	}
}

// outputJSON outputs data as JSON
func outputJSON(data interface{}) error {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonBytes))
	return nil
}

// newRegistryCommand creates the registry subcommand group
func newRegistryCommand(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage artifact registries",
		Long:  `Commands to manage Harness Artifact Registries`,
	}

	// Add registry subcommands
	cmd.AddCommand(newListRegistriesCommand(config))
	cmd.AddCommand(newGetRegistryCommand(config))
	cmd.AddCommand(newCreateRegistryCommand(config))
	cmd.AddCommand(newDeleteRegistryCommand(config))

	return cmd
}

// newListRegistriesCommand creates command to list registries
func newListRegistriesCommand(config APIConfig) *cobra.Command {
	var packageTypes []string
	var registryType string
	var pageNumber, pageSize int
	var searchTerm string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifact registries",
		Long:  `List all artifact registries in the account or space`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			
			// Create API client
			client := createClient(config)
			
			// With a real implementation, we would call the generated client's ListRegistries method
			// For now, we mock the response
			fmt.Println("Listing registries with format:", format)
			fmt.Println("This is a simulated response. In a real implementation, we would call the API.")
			
			// Return mock data based on specified format
			if format == "json" {
				fmt.Println(`{"registries": [{ "identifier": "docker-hub", "type": "DOCKER", "url": "https://registry.example.com" }]}`)
			} else {
				fmt.Println("IDENTIFIER\tTYPE\tURL")
				fmt.Println("docker-hub\tDOCKER\thttps://registry.example.com")
			}
			
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&packageTypes, "package-type", nil, "Package type filter (DOCKER, MAVEN, etc.)")
	cmd.Flags().StringVar(&registryType, "registry-type", "", "Registry type (VIRTUAL, UPSTREAM)")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number for pagination")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size for pagination")
	cmd.Flags().StringVar(&searchTerm, "search", "", "Search term to filter registries")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "Whether to list registries recursively")

	return cmd
}

// newGetRegistryCommand creates command to get registry details
func newGetRegistryCommand(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [registry-identifier]",
		Short: "Get registry details",
		Long:  `Get details of a specific registry`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			regID := args[0]
			format, _ := cmd.Flags().GetString("format")
			
			// Create API client
			client := createClient(config)
			
			// With a real implementation, we would call the generated client
			// For now, we mock the response
			fmt.Printf("Getting registry %s with format: %s\n", regID, format)
			
			// Return mock data based on specified format
			if format == "json" {
				fmt.Printf(`{"identifier": "%s", "type": "DOCKER", "url": "https://registry.example.com/%s" }`, regID, regID)
			} else {
				fmt.Println("IDENTIFIER\tTYPE\tURL")
				fmt.Printf("%s\tDOCKER\thttps://registry.example.com/%s\n", regID, regID)
			}
			
			return nil
		},
	}

	return cmd
}

// newCreateRegistryCommand creates command to create a registry
func newCreateRegistryCommand(config APIConfig) *cobra.Command {
	var identifier, description, packageType, registryType string
	var labels []string
	
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create artifact registry",
		Long:  `Create a new artifact registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			
			// Create API client
			client := createClient(config)
			
			// With a real implementation, we would call the generated client
			// For now, we mock the response
			fmt.Printf("Creating registry %s of type %s with format: %s\n", identifier, packageType, format)
			
			// Return success message
			fmt.Println("Registry created successfully!")
			
			return nil
		},
	}
	
	cmd.Flags().StringVar(&identifier, "identifier", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&description, "description", "", "Registry description")
	cmd.Flags().StringVar(&packageType, "package-type", "", "Package type (DOCKER, MAVEN, etc.) (required)")
	cmd.Flags().StringVar(&registryType, "registry-type", "", "Registry type (VIRTUAL, UPSTREAM) (required)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Registry labels")
	
	cmd.MarkFlagRequired("identifier")
	cmd.MarkFlagRequired("package-type")
	cmd.MarkFlagRequired("registry-type")
	
	return cmd
}

// newDeleteRegistryCommand creates command to delete a registry
func newDeleteRegistryCommand(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [registry-identifier]",
		Short: "Delete artifact registry",
		Long:  `Delete an artifact registry by its identifier`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			regID := args[0]
			
			// Create API client
			client := createClient(config)
			
			// With a real implementation, we would call the generated client
			// For now, we mock the response
			fmt.Printf("Deleting registry %s\n", regID)
			
			// Return success message
			fmt.Println("Registry deleted successfully!")
			
			return nil
		},
	}

	return cmd
}

// newArtifactCommand creates the artifact subcommand group
func newArtifactCommand(config APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage artifacts",
		Long:  `Commands to manage artifacts in registries`,
	}

	// Add artifact subcommands
	cmd.AddCommand(newListArtifactsCommand(config))
	cmd.AddCommand(newGetArtifactCommand(config))

	return cmd
}

// newListArtifactsCommand creates command to list artifacts
func newListArtifactsCommand(config APIConfig) *cobra.Command {
	var registryID string
	var pageNumber, pageSize int
	var searchTerm string
	var latestVersion, deployedArtifact bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts",
		Long:  `List artifacts in a registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			
			// Create API client
			client := createClient(config)
			
			// With a real implementation, we would call the generated client
			// For now, we mock the response
			fmt.Printf("Listing artifacts in registry %s with format: %s\n", registryID, format)
			
			// Return mock data based on specified format
			if format == "json" {
				fmt.Println(`{"artifacts": [{ "name": "nginx", "latestVersion": "1.21.6", "downloadCount": 1250 }]}`)
			} else {
				fmt.Println("NAME\tLATEST VERSION\tDOWNLOADS")
				fmt.Println("nginx\t1.21.6\t1250")
			}
			
			return nil
		},
	}

	cmd.Flags().StringVar(&registryID, "registry", "", "Registry identifier (required)")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number for pagination")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size for pagination")
	cmd.Flags().StringVar(&searchTerm, "search", "", "Search term to filter artifacts")
	cmd.Flags().BoolVar(&latestVersion, "latest", false, "Show only latest versions")
	cmd.Flags().BoolVar(&deployedArtifact, "deployed", false, "Show only deployed artifacts")
	
	cmd.MarkFlagRequired("registry")

	return cmd
}

// newGetArtifactCommand creates command to get artifact details
func newGetArtifactCommand(config APIConfig) *cobra.Command {
	var registryID string

	cmd := &cobra.Command{
		Use:   "get [artifact-name]",
		Short: "Get artifact details",
		Long:  `Get details of a specific artifact`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactName := args[0]
			format, _ := cmd.Flags().GetString("format")
			
			// Create API client
			client := createClient(config)
			
			// With a real implementation, we would call the generated client
			// For now, we mock the response
			fmt.Printf("Getting artifact %s from registry %s with format: %s\n", artifactName, registryID, format)
			
			// Return mock data based on specified format
			if format == "json" {
				fmt.Printf(`{"name": "%s", "latestVersion": "1.0.0", "downloadCount": 350 }`, artifactName)
			} else {
				fmt.Println("NAME\tLATEST VERSION\tDOWNLOADS")
				fmt.Printf("%s\t1.0.0\t350\n", artifactName)
			}
			
			return nil
		},
	}

	cmd.Flags().StringVar(&registryID, "registry", "", "Registry identifier (required)")
	cmd.MarkFlagRequired("registry")

	return cmd
}

// newRegistryCommand creates the registry subcommand group
func newRegistryCommand(config *APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage artifact registries",
		Long:  `Commands to manage Harness Artifact Registries`,
	}

	// Add registry subcommands
	cmd.AddCommand(newListRegistriesCommand(config))
	cmd.AddCommand(newGetRegistryCommand(config))
	cmd.AddCommand(newCreateRegistryCommand(config))
	cmd.AddCommand(newUpdateRegistryCommand(config))
	cmd.AddCommand(newDeleteRegistryCommand(config))

	return cmd
}

// newListRegistriesCommand creates the registry list subcommand
func newListRegistriesCommand(config *APIConfig) *cobra.Command {
	var spaceRef string
	var pageNumber, pageSize int
	var packageType, registryType []string
	var searchTerm string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifact registries",
		Long:  `List all artifact registries in the account`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			// When we integrate with the generated client, we'll use the client
			// For now, we'll use the mock implementation
			return listRegistries(spaceRef, packageType, registryType, searchTerm, recursive, pageNumber, pageSize, format, config)
		},
	}

	cmd.Flags().StringVar(&spaceRef, "space", "", "Space reference")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size")
	cmd.Flags().StringSliceVar(&packageType, "package-type", nil, "Package type filter (can specify multiple)")
	cmd.Flags().StringSliceVar(&registryType, "registry-type", nil, "Registry type filter (can specify multiple)")
	cmd.Flags().StringVar(&searchTerm, "search", "", "Search term to filter registries")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "Whether to list registries recursively")

	return cmd
}

// newGetRegistryCommand creates the registry get subcommand
func newGetRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [registry-identifier]",
		Short: "Get registry details",
		Long:  `Get details of a specific artifact registry`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return getRegistry(args[0], format)
		},
	}
	return cmd
}

// newCreateRegistryCommand creates the registry create subcommand
func newCreateRegistryCommand() *cobra.Command {
	var identifier, description, packageType, registryType, spaceRef string
	var labels []string
	
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create artifact registry",
		Long:  `Create a new artifact registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return createRegistry(identifier, description, packageType, registryType, labels, spaceRef, format)
		},
	}
	
	cmd.Flags().StringVar(&identifier, "identifier", "", "Registry identifier (required)")
	cmd.Flags().StringVar(&description, "description", "", "Registry description")
	cmd.Flags().StringVar(&packageType, "package-type", "", "Package type (DOCKER, MAVEN, GENERIC, HELM) (required)")
	cmd.Flags().StringVar(&registryType, "registry-type", "", "Registry type (VIRTUAL, UPSTREAM) (required)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Registry labels")
	cmd.Flags().StringVar(&spaceRef, "space", "", "Space reference")
	cmd.MarkFlagRequired("identifier")
	cmd.MarkFlagRequired("package-type")
	cmd.MarkFlagRequired("registry-type")
	
	return cmd
}

// newUpdateRegistryCommand creates the registry update subcommand
func newUpdateRegistryCommand() *cobra.Command {
	var description string
	var labels []string
	
	cmd := &cobra.Command{
		Use:   "update [registry-identifier]",
		Short: "Update artifact registry",
		Long:  `Update an existing artifact registry`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return updateRegistry(args[0], description, labels, format)
		},
	}
	
	cmd.Flags().StringVar(&description, "description", "", "Registry description")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Registry labels")
	
	return cmd
}

// newDeleteRegistryCommand creates the registry delete subcommand
func newDeleteRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [registry-identifier]",
		Short: "Delete artifact registry",
		Long:  `Delete an artifact registry by its identifier`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteRegistry(args[0])
		},
	}
	return cmd
}

// newArtifactCommand creates the artifact subcommand group
func newArtifactCommand(config *APIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage artifacts",
		Long:  `Commands to manage artifacts in registries`,
	}

	// Add artifact subcommands
	cmd.AddCommand(newListArtifactsCommand(config))
	cmd.AddCommand(newGetArtifactCommand(config))
	cmd.AddCommand(newDeleteArtifactCommand(config))
	cmd.AddCommand(newLabelArtifactCommand(config))
	cmd.AddCommand(newArtifactVersionCommand(config))
	
	return cmd
}

// newListArtifactsCommand creates the artifact list subcommand
func newListArtifactsCommand() *cobra.Command {
	var registryRef string
	var pageNumber, pageSize int
	var searchTerm string
	var latestVersion, deployedArtifact bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts",
		Long:  `List artifacts in a registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return listArtifacts(registryRef, searchTerm, latestVersion, deployedArtifact, pageNumber, pageSize, format)
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.Flags().StringVar(&searchTerm, "search", "", "Search term")
	cmd.Flags().BoolVar(&latestVersion, "latest", false, "Show only latest versions")
	cmd.Flags().BoolVar(&deployedArtifact, "deployed", false, "Show only deployed artifacts")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size")
	cmd.MarkFlagRequired("registry")

	return cmd
}

// newGetArtifactCommand creates the artifact get subcommand
func newGetArtifactCommand() *cobra.Command {
	var registryRef string

	cmd := &cobra.Command{
		Use:   "get [artifact-name]",
		Short: "Get artifact details",
		Long:  `Get summary details of a specific artifact`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return getArtifactSummary(registryRef, args[0], format)
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.MarkFlagRequired("registry")

	return cmd
}

// newDeleteArtifactCommand creates the artifact delete subcommand
func newDeleteArtifactCommand() *cobra.Command {
	var registryRef string

	cmd := &cobra.Command{
		Use:   "delete [artifact-name]",
		Short: "Delete artifact",
		Long:  `Delete an artifact by its name`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteArtifact(registryRef, args[0])
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.MarkFlagRequired("registry")

	return cmd
}

// newLabelArtifactCommand creates the artifact label subcommand
func newLabelArtifactCommand() *cobra.Command {
	var registryRef string
	var labels []string

	cmd := &cobra.Command{
		Use:   "label [artifact-name]",
		Short: "Update artifact labels",
		Long:  `Update labels for an artifact`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return updateArtifactLabels(registryRef, args[0], labels, format)
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Artifact labels (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("labels")

	return cmd
}

// newArtifactVersionCommand creates the artifact version subcommand group
func newArtifactVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Manage artifact versions",
		Long:  `Commands to manage artifact versions`,
	}

	// Add version subcommands
	cmd.AddCommand(newListVersionsCommand())
	cmd.AddCommand(newGetVersionCommand())
	cmd.AddCommand(newDeleteVersionCommand())

	return cmd
}

// newListVersionsCommand creates the version list subcommand
func newListVersionsCommand() *cobra.Command {
	var registryRef, artifactName string
	var pageNumber, pageSize int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifact versions",
		Long:  `List versions of a specific artifact`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return listArtifactVersions(registryRef, artifactName, pageNumber, pageSize, format)
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.Flags().StringVar(&artifactName, "artifact", "", "Artifact name (required)")
	cmd.Flags().IntVar(&pageNumber, "page", 1, "Page number")
	cmd.Flags().IntVar(&pageSize, "size", 20, "Page size")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("artifact")

	return cmd
}

// newGetVersionCommand creates the version get subcommand
func newGetVersionCommand() *cobra.Command {
	var registryRef, artifactName string
	var childVersion string

	cmd := &cobra.Command{
		Use:   "get [version]",
		Short: "Get artifact version details",
		Long:  `Get details of a specific artifact version`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			return getArtifactVersionDetails(registryRef, artifactName, args[0], childVersion, format)
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.Flags().StringVar(&artifactName, "artifact", "", "Artifact name (required)")
	cmd.Flags().StringVar(&childVersion, "child-version", "", "Child version (for Docker artifacts)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("artifact")

	return cmd
}

// newDeleteVersionCommand creates the version delete subcommand
func newDeleteVersionCommand() *cobra.Command {
	var registryRef, artifactName string

	cmd := &cobra.Command{
		Use:   "delete [version]",
		Short: "Delete artifact version",
		Long:  `Delete a specific version of an artifact`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteArtifactVersion(registryRef, artifactName, args[0])
		},
	}

	cmd.Flags().StringVar(&registryRef, "registry", "", "Registry reference (required)")
	cmd.Flags().StringVar(&artifactName, "artifact", "", "Artifact name (required)")
	cmd.MarkFlagRequired("registry")
	cmd.MarkFlagRequired("artifact")

	return cmd
}
