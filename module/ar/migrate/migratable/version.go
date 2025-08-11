package migratable

import (
	"context"
	"fmt"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Version struct {
	srcRegistry  string
	destRegistry string
	srcAdapter   adapter.Adapter
	destAdapter  adapter.Adapter
	artifactType types.ArtifactType
	logger       zerolog.Logger
	pkg          types.Package
	version      types.Version
	node         *types.TreeNode
	stats        *types.TransferStats
	mapping      *types.RegistryMapping
	concurrency  int
}

func NewVersionJob(
	src adapter.Adapter,
	dest adapter.Adapter,
	srcRegistry string,
	destRegistry string,
	artifactType types.ArtifactType,
	pkg types.Package,
	version types.Version,
	node *types.TreeNode,
	stats *types.TransferStats,
	mapping *types.RegistryMapping,
	concurrency int,
) engine.Job {
	jobID := uuid.New().String()

	jobLogger := log.With().
		Str("job_type", "version").
		Str("job_id", jobID).
		Str("source_registry", srcRegistry).
		Str("dest_registry", destRegistry).
		Str("package", pkg.Name).
		Str("version", version.Name).
		Logger()

	return &Version{
		srcRegistry:  srcRegistry,
		destRegistry: destRegistry,
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: artifactType,
		logger:       jobLogger,
		pkg:          pkg,
		version:      version,
		node:         node,
		stats:        stats,
		mapping:      mapping,
		concurrency:  concurrency,
	}
}

func (r *Version) Info() string {
	return r.pkg.Name
}

func (r *Version) Pre(ctx context.Context) error {
	// Extract trace ID from context if available
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "pre").
		Str("trace_id", traceID).
		Logger()
	logger.Info().Msg("Starting version pre-migration step")
	startTime := time.Now()

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed version pre-migration step")
	return nil
}

func (r *Version) Migrate(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "migrate").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting version migration step")
	startTime := time.Now()

	var jobs []engine.Job

	if r.artifactType == types.GENERIC || r.artifactType == types.MAVEN || r.artifactType == types.PYTHON ||
		r.artifactType == types.NUGET || r.artifactType == types.NPM {
		files, err := tree.GetAllFiles(r.node)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get files from tree")
			return fmt.Errorf("get files from tree failed: %w", err)
		}
		for _, file := range files {
			job := NewFileJob(r.srcAdapter, r.destAdapter, r.srcRegistry, r.destRegistry, r.artifactType, r.pkg,
				r.version, r.node, file, r.stats, r.mapping, r.concurrency)
			jobs = append(jobs, job)
		}
	}

	if r.artifactType == types.DOCKER || r.artifactType == types.HELM || r.artifactType == types.HELM_LEGACY {
		log.Error().Ctx(ctx).Msgf("OCI migrate version is not supported")
		return fmt.Errorf("OCI migrate version is not supported")
	}

	log.Info().Msgf("Jobs length: %d", len(jobs))

	eng := engine.NewEngine(r.concurrency, jobs)
	err := eng.Execute(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Engine execution failed")
		return fmt.Errorf("engine execution failed: %w", err)
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed version migration step")
	return nil
}

// Post Any post processing work
func (r *Version) Post(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "post").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting version post-migration step")

	startTime := time.Now()
	// Your post-migration code here

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed version post-migration step")
	return nil
}
