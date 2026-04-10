package command

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	client2 "github.com/harness/harness-cli/util/client"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDeleteArtifactCmd(c *cmdutils.Factory) *cobra.Command {
	var name, registry, version, configPath string
	cmd := &cobra.Command{
		Use:   "delete [artifact-name]",
		Short: "Delete an artifact or a specific version",
		Long:  "Deletes an artifact and all its versions, or a specific version if --version flag is provided. Use 'all' with --config flag for bulk delete.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]

			// If config file provided from flag is provided, only then execute bulk delete
			if configPath != "" {
				executeBulkDelete(configPath)
				return nil
			}
			registryRef := client2.GetRef(client2.GetScopeRef(), registry)
			// Otherwise, we will execute old normal flow
			// If version flag is provided, delete specific version
			if version != "" {
				return performVersionDelete(c, registryRef, name, version)
			}

			// Otherwise, delete entire artifact (all versions)
			return performArtifactDelete(c, registryRef, name)

		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry (required for normal delete, not needed with --config)")
	cmd.Flags().StringVar(&version, "version", "", "specific version to delete (if not provided, deletes all versions)")
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to bulk delete configuration file")

	// Make registry required only when config is not provided
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if configPath == "" && registry == "" {
			return fmt.Errorf("--registry flag is required when --config is not provided")
		}
		return nil
	}

	return cmd
}

func performVersionDelete(c *cmdutils.Factory, registryRef, name, version string) error {
	response, err := c.RegistryHttpClient().DeleteArtifactVersionWithResponse(context.Background(), registryRef, name, version)
	if err != nil {
		return err
	}
	if response.JSON200 != nil {
		log.Info().Msgf("Deleted artifact version %s/%s; msg:%s", name, version, response.JSON200.Status)
	} else {
		log.Error().Msgf("failed to delete artifact version %s/%s %s", name, version, string(response.Body))
	}
	return nil
}

func performArtifactDelete(c *cmdutils.Factory, registryRef, name string) error {
	response, err := c.RegistryHttpClient().DeleteArtifactWithResponse(context.Background(), registryRef, name)
	if err != nil {
		return err
	}
	if response.JSON200 != nil {
		log.Info().Msgf("Deleted artifact %s and all its versions; msg:%s", name, response.JSON200.Status)
	} else {
		log.Error().Msgf("failed to delete artifact %s %s", name, string(response.Body))
	}
	return nil
}

func executeBulkDelete(configPath string) {
	if configPath == "" {
		return
	}

	config, err := utils.LoadBulkDeleteConfig(configPath)
	if err != nil {
		log.Error().Msgf("Failed to load bulk delete config: %v", err)
		return
	}

	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Error().Msgf("Failed to marshal config to JSON: %v", err)
		return
	}

	log.Info().Msgf("Bulk delete configuration loaded from %s", configPath)
	fmt.Println("=== Bulk Delete Configuration ===")
	log.Info().Msgf(string(configJSON))
	log.Info().Msgf("\n=== Detailed Registry Information ===")

	for registryName, packages := range config.Registries {
		log.Info().Msgf("\nRegistry: %s\n", registryName)
		for packageName, versions := range packages {
			log.Info().Msgf("  Package: %s\n", packageName)
			if len(versions) == 0 {
				log.Info().Msgf("    Versions: [] (delete all versions)\n")
				//call version delete API
			} else {
				fmt.Printf("    Versions: %v\n", versions)
				// call package delete API , API Is same but  parameter is different
			}
		}
	}
}
