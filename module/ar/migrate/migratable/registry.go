package migratable

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/engine"
	"harness/module/ar/migrate/tree"
	"harness/module/ar/migrate/types"
)

type Registry struct {
	srcRegistry  string
	destRegistry string
	srcAdapter   adapter.Adapter
	destAdapter  adapter.Adapter
	artifactType types.ArtifactType
	logger       zerolog.Logger
	stats        *types.TransferStats
}

func NewRegistryJob(
	src adapter.Adapter,
	dest adapter.Adapter,
	srcRegistry string,
	destRegistry string,
	artifactType types.ArtifactType,
	stats *types.TransferStats,
) engine.Job {
	jobID := uuid.New().String()

	jobLogger := log.With().
		Str("job_type", "registry").
		Str("job_id", jobID).
		Str("source_registry", srcRegistry).
		Str("dest_registry", destRegistry).
		Logger()

	return &Registry{
		srcRegistry:  srcRegistry,
		destRegistry: destRegistry,
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: artifactType,
		logger:       jobLogger,
		stats:        stats,
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

	_, err := r.destAdapter.CreateRegistryIfDoesntExist(r.destRegistry)
	if err != nil {
		logger.Error().
			Err(err).
			Dur("duration", time.Since(startTime)).
			Msg("Failed to create registry")
		return fmt.Errorf("create registry failed: %w", err)
	}

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

	startTime := time.Now()

	files, err2 := r.srcAdapter.GetFiles(r.srcRegistry)
	if err2 != nil {
		logger.Error().Msgf("Failed to get files from registry %s", r.srcRegistry)
		return fmt.Errorf("get files from registry %s failed: %w", r.srcRegistry, err2)
	}
	root := tree.TransformToTree(files)

	pkgs, err := r.srcAdapter.GetPackages(r.srcRegistry, r.artifactType, root)
	if err != nil {
		logger.Error().Msg("Failed to get packages")
		return fmt.Errorf("get packages failed: %w", err)
	}

	var jobs []engine.Job
	for _, pkg := range pkgs {
		treeNode, err2 := tree.GetNodeForPath(root, pkg.Path)
		if err2 != nil {
			logger.Error().Msgf("Failed to get node for path %s", pkg.Path)
			return fmt.Errorf("get node for path %s failed: %w", pkg.Path, err2)
		}
		job := NewPackageJob(r.srcAdapter, r.destAdapter, r.srcRegistry, r.destRegistry, r.artifactType, pkg, treeNode,
			r.stats)
		jobs = append(jobs, job)
	}

	eng := engine.NewEngine(5, jobs)
	err = eng.Execute(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Engine execution failed")
		return fmt.Errorf("engine execution failed: %w", err)
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
