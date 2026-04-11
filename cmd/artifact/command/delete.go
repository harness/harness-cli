package command

import (
	"context"
	"fmt"
	"sync"

	"github.com/harness/harness-cli/cmd/artifact/command/utils"
	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	client2 "github.com/harness/harness-cli/util/client"
	p "github.com/harness/harness-cli/util/common/progress"

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

			progress := p.NewConsoleReporter()
			// If config file provided from flag is provided, only then execute bulk delete
			if configPath != "" {
				progress.Start("Found Config file for bulk delete ")
				executeBulkDelete(c, configPath, progress)
				return nil
			}
			registryRef := client2.GetRef(client2.GetScopeRef(), registry)
			// Otherwise, we will execute old normal flow
			// If version flag is provided, delete specific version
			if version != "" {
				progress.Step("deleting the provided version ")
				return performVersionDelete(c, registryRef, name, version)
			}

			// Otherwise, delete entire artifact (all versions)
			progress.Step("deleting the provided artifact ")
			return performArtifactDelete(c, registryRef, name)

		},
	}

	// Common flags
	cmd.Flags().StringVar(&registry, "registry", "", "name of the registry (required for normal delete, not needed with --config)")
	cmd.Flags().StringVar(&version, "version", "", "specific version to delete (if not provided, deletes all versions)")
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to bulk delete configuration file")

	// forcing registry required only when config is not provided AH-2948
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

func executeBulkDelete(c *cmdutils.Factory, configPath string, progress *p.ConsoleReporter) {
	if configPath == "" {
		return
	}

	deleteConfig, err := utils.LoadBulkDeleteConfig(configPath)
	if err != nil {
		progress.Error("Failed to load bulk delete config ")
		log.Error().Msgf("Failed to load bulk delete config: %v", err)
		return
	}

	scopeRef := client2.GetScopeRef()
	if deleteConfig.OrgID != "" && deleteConfig.ProjectID != "" {
		log.Info().Msgf("creating scopeRef using  orgID and projectID provided in bulk delete config file")
		scopeRef = client2.GetRef(config.Global.AccountID, deleteConfig.OrgID, deleteConfig.ProjectID)
	}
	// Collect all deletion jobs
	var jobs []deleteJob
	for registryName, packages := range deleteConfig.Registries {

		registryRef := client2.GetRef(scopeRef, registryName)

		for packageName, versions := range packages {

			if len(versions) == 0 {
				// creating  Artifact delete job
				jobs = append(jobs, deleteJob{
					jobType:      "artifact",
					registryName: registryName,
					registryRef:  registryRef,
					packageName:  packageName,
				})
			} else {
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
	progress.Step("Starting bulk delete ..")
	log.Info().Msgf("\nTotal deletion jobs collected: %d", len(jobs))
	log.Info().Msgf("Starting concurrent deletion with concurrency: %d\n", deleteConfig.Concurrency)

	// Execute deletions concurrently
	processDeletionsConcurrently(c, jobs, deleteConfig.Concurrency)
	progress.Step("Bulk delete execution complete..")
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
						log.Error().Msgf("Failed to delete version : %v", err)
					}
				} else if job.jobType == "artifact" {
					log.Info().Msgf("[Worker %d] Deleting artifact %s (all versions) from registry %s",
						workerID, job.packageName, job.registryName)
					err := performArtifactDelete(c, job.registryRef, job.packageName)
					if err != nil {
						log.Error().Msgf("Failed to Delete artifact: %v", err)
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

	log.Info().Msgf("Bulk deletion completed.")
}

type deleteJob struct {
	jobType      string // "version" or "artifact"
	registryName string
	registryRef  string
	packageName  string
	version      string // empty for artifact deletion
}
