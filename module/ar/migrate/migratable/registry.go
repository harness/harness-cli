package migratable

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Registry struct {
	srcRegistry           string
	sourcePackageHostname string
	destRegistry          string
	srcAdapter            adapter.Adapter
	destAdapter           adapter.Adapter
	artifactType          types.ArtifactType
	logger                zerolog.Logger
	stats                 *types.TransferStats
	mapping               *types.RegistryMapping
	config                *types.Config

	// Transient
	registry types.RegistryInfo
}

func NewRegistryJob(
	src adapter.Adapter,
	dest adapter.Adapter,
	srcRegistry string,
	sourcePackageHostname string,
	destRegistry string,
	artifactType types.ArtifactType,
	stats *types.TransferStats,
	mapping *types.RegistryMapping,
	config *types.Config,
) engine.Job {
	jobID := uuid.New().String()

	jobLogger := log.With().
		Str("job_type", "registry").
		Str("job_id", jobID).
		Str("source_registry", srcRegistry).
		Str("dest_registry", destRegistry).
		Logger()

	return &Registry{
		srcRegistry:           srcRegistry,
		sourcePackageHostname: sourcePackageHostname,
		destRegistry:          destRegistry,
		srcAdapter:            src,
		destAdapter:           dest,
		artifactType:          artifactType,
		logger:                jobLogger,
		stats:                 stats,
		mapping:               mapping,
		config:                config,
	}
}

func (r *Registry) Info() string {
	return r.srcRegistry + ":" + r.destRegistry
}

// Pre Create registry at destination if it doesn't exist
func (r *Registry) Pre(ctx context.Context) error {
	// Extract trace ID from context if available
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "pre").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting registry pre-migration step")

	startTime := time.Now()
	registry, err := r.destAdapter.GetRegistry(ctx, r.destRegistry)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to get registry %q", r.destRegistry)
		return fmt.Errorf("failed to get registry %q", r.destRegistry)
	}

	log.Info().Ctx(ctx).Msgf("Found registry %+v", registry)
	r.registry = registry

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed registry pre-migration step")
	return nil
}

// Migrate Create down stream packages and migrate them
func (r *Registry) Migrate(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "migrate").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting registry migration step")

	if len(r.mapping.IncludePatterns) > 0 && len(r.mapping.ExcludePatterns) > 0 {
		logger.Error().Msgf("Either include or Exclude Pattern is suppoted at a time for %s", r.artifactType)
		return fmt.Errorf("failed in validating config file for %s ", r.artifactType)
	}

	startTime := time.Now()

	files, err2 := r.srcAdapter.GetFiles(r.srcRegistry)
	if err2 != nil {
		logger.Error().Msgf("Failed to get files from registry %s", r.srcRegistry)
		return fmt.Errorf("get files from registry %s failed: %w", r.srcRegistry, err2)
	}

	// Filter files based on include/exclude patterns
	currArtifactType := r.artifactType
	if util.IsFileLevelFilterableArtifact(currArtifactType) {
		if len(r.mapping.IncludePatterns) > 0 || len(r.mapping.ExcludePatterns) > 0 {
			originalCount := len(files)
			filteredFiles := util.FilterFilesByPatterns(files, r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
			files = filteredFiles
			logger.Info().Msgf("Filtered files: %d -> %d (includePatterns: %v, excludePatterns: %v)",
				originalCount, len(files), r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
		}
	}

	root := tree.TransformToTree(files)

	pkgs, err := r.srcAdapter.GetPackages(r.srcRegistry, r.artifactType, root)
	if err != nil {
		logger.Error().Msg("Failed to get packages")
		return fmt.Errorf("get packages failed: %w", err)
	}

	// applying package level filter
	if util.IsPackageLevelFilterableArtifact(currArtifactType) {
		if len(r.mapping.IncludePatterns) > 0 || len(r.mapping.ExcludePatterns) > 0 {
			originalCount := len(pkgs)
			filteredPackages := util.FilterFilesByPatternsPackageName(pkgs, r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
			pkgs = filteredPackages
			logger.Info().Msgf("Filtered packages: %d -> %d (includePatterns: %v, excludePatterns: %v)",
				originalCount, len(pkgs), r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
		}
	}

	var jobs []engine.Job
	for _, pkg := range pkgs {
		treeNode, err2 := tree.GetNodeForPath(root, pkg.Path)
		if err2 != nil {
			logger.Error().Msgf("Failed to get node for path %s", pkg.Path)
			return fmt.Errorf("get node for path %s failed: %w", pkg.Path, err2)
		}
		job := NewPackageJob(r.srcAdapter, r.destAdapter, r.srcRegistry, r.sourcePackageHostname, r.destRegistry, r.artifactType, pkg, treeNode,
			r.stats, r.mapping, r.config, r.registry)
		jobs = append(jobs, job)
	}

	eng := engine.NewEngine(r.config.Concurrency, jobs)
	err = eng.Execute(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Engine execution saw following errors")
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed registry migration step")
	return nil
}

// Post Any post processing work
func (r *Registry) Post(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "post").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting registry post-migration step")

	startTime := time.Now()
	// Your post-migration code here

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed registry post-migration step")
	return nil
}
