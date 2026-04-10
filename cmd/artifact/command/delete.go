package command

import (
	"context"
	"fmt"
	"sync"

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
				executeBulkDelete(c, configPath)
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

	fmt.Println(registryRef)
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

func executeBulkDelete(c *cmdutils.Factory, configPath string) {
	if configPath == "" {
		return
	}

	config, err := utils.LoadBulkDeleteConfig(configPath)
	if err != nil {
		log.Error().Msgf("Failed to load bulk delete config: %v", err)
		return
	}

	/*configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Error().Msgf("Failed to marshal config to JSON: %v", err)
		return
	}

	log.Info().Msgf("Bulk delete configuration loaded from %s", configPath)
	log.Info().Msgf(string(configJSON))
	log.Info().Msgf("\n=== Detailed Registry Information ===")

	*/

	// Collect all deletion jobs
	var jobs []deleteJob
	for registryName, packages := range config.Registries {
		//log.Info().Msgf("\nRegistry: %s\n", registryName)
		registryRef := client2.GetRef(client2.GetScopeRef(), registryName)

		for packageName, versions := range packages {
			//log.Info().Msgf("  Package: %s\n", packageName)
			if len(versions) == 0 {
				//log.Info().Msgf("    Versions: [] (delete all versions)\n")
				// creating complete Artifact delete job
				jobs = append(jobs, deleteJob{
					jobType:      "artifact",
					registryName: registryName,
					registryRef:  registryRef,
					packageName:  packageName,
				})
			} else {
				log.Info().Msgf("    Versions: %v\n", versions)
				// creating version delete job
				for _, version := range versions {
					jobs = append(jobs, deleteJob{
						jobType:      "version",
						registryName: registryName,
						registryRef:  registryRef,
						packageName:  packageName,
						version:      version,
					})
				}
			}
		}
	}

	log.Info().Msgf("\nTotal deletion jobs collected: %d", len(jobs))
	log.Info().Msgf("Starting concurrent deletion with concurrency: %d\n", config.Concurrency)

	// Execute deletions concurrently
	processDeletionsConcurrently(c, jobs, config.Concurrency)
}

func processDeletionsConcurrently(c *cmdutils.Factory, jobs []deleteJob, concurrency int) {
	jobChan := make(chan deleteJob, len(jobs))
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobChan {
				if job.jobType == "version" {
					log.Info().Msgf("[Worker %d] Deleting version %s : %s from registry %s",
						workerID, job.packageName, job.version, job.registryName)
					err := performVersionDelete(c, job.registryRef, job.packageName, job.version)
					if err != nil {
						log.Error().Msgf("[Worker %d] Failed to delete version %s : %s - %v",
							workerID, job.packageName, job.version, err)
					} else {
						log.Info().Msgf("[Worker %d] Successfully deleted version %s :%s",
							workerID, job.packageName, job.version)
					}
				} else if job.jobType == "artifact" {
					log.Info().Msgf("[Worker %d] Deleting artifact %s (all versions) from registry %s",
						workerID, job.packageName, job.registryName)
					err := performArtifactDelete(c, job.registryRef, job.packageName)
					if err != nil {
						log.Error().Msgf("[Worker %d] Failed to delete artifact %s - %v",
							workerID, job.packageName, err)
					} else {
						log.Info().Msgf("[Worker %d] Successfully deleted artifact %s",
							workerID, job.packageName)
					}
				}
			}
		}(i + 1)
	}

	// Send jobs to channel
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for all workers to complete
	wg.Wait()
	log.Info().Msgf("\nBulk deletion completed. Processed %d jobs.", len(jobs))
}

type deleteJob struct {
	jobType      string // "version" or "artifact"
	registryName string
	registryRef  string
	packageName  string
	version      string // empty for artifact deletion
}
