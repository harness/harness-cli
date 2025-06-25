package migratable

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/uuid"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/engine"
	"harness/module/ar/migrate/lib"
	"harness/module/ar/migrate/tree"
	"harness/module/ar/migrate/types"
)

type Package struct {
	srcRegistry  string
	destRegistry string
	srcAdapter   adapter.Adapter
	destAdapter  adapter.Adapter
	artifactType types.ArtifactType
	logger       zerolog.Logger
	pkg          types.Package
	node         *types.TreeNode
	stats        *types.TransferStats
}

func NewPackageJob(
	src adapter.Adapter,
	dest adapter.Adapter,
	srcRegistry string,
	destRegistry string,
	artifactType types.ArtifactType,
	pkg types.Package,
	node *types.TreeNode,
	stats *types.TransferStats,
) engine.Job {
	jobID := uuid.New().String()

	jobLogger := log.With().
		Str("job_type", "package").
		Str("job_id", jobID).
		Str("source_registry", srcRegistry).
		Str("dest_registry", destRegistry).
		Str("package", pkg.Name).
		Logger()

	return &Package{
		srcRegistry:  srcRegistry,
		destRegistry: destRegistry,
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: artifactType,
		logger:       jobLogger,
		pkg:          pkg,
		node:         node,
		stats:        stats,
	}
}

func (r *Package) Info() string {
	return r.pkg.Name
}

// Pre Create package at destination if it doesn't exist
func (r *Package) Pre(ctx context.Context) error {
	// Extract trace ID from context if available
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "pre").
		Str("trace_id", traceID).
		Logger()
	logger.Info().Msg("Starting package pre-migration step")
	startTime := time.Now()

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed registry pre-migration step")
	return nil
}

// Migrate Create down stream packages and migrate them
func (r *Package) Migrate(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "migrate").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting registry migration step")

	startTime := time.Now()

	if r.artifactType == types.DOCKER || r.artifactType == types.HELM {
		srcImage, _ := r.srcAdapter.GetOCIImagePath(r.srcRegistry, r.pkg.Name)
		dstImage, _ := r.destAdapter.GetOCIImagePath(r.destRegistry, r.pkg.Name)
		pterm.Info.Println(fmt.Sprintf("Copying repository %s to %s", srcImage, dstImage))
		logger.Info().Ctx(ctx).Msgf("Copying repository %s to %s", srcImage, dstImage)
		err := crane.CopyRepository(
			srcImage,
			dstImage,
			crane.WithUserAgent("harness-cli"),
			crane.WithContext(ctx),
			crane.WithJobs(4),
			crane.WithNoClobber(true),
			crane.WithAuthFromKeychain(lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.srcRegistry,
				r.destRegistry)),
		)
		stat := types.FileStat{
			Name:     r.pkg.Name,
			Registry: r.srcRegistry,
			Uri:      srcImage,
			Size:     0,
			Status:   types.StatusSuccess,
		}
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("Failed to copy repository %s to %s %v", srcImage, dstImage, err)
			pterm.Error.Println(fmt.Sprintf("Failed to copy repository %s to %s", srcImage, dstImage))
			stat.Error = err.Error()
		} else {
			pterm.Success.Println(fmt.Sprintf("Copy repository %s to %s completed", srcImage, dstImage))
		}
		r.stats.FileStats = append(r.stats.FileStats, stat)
	} else {
		versions, err := r.srcAdapter.GetVersions(r.srcRegistry, r.pkg.Name, r.artifactType)
		if err != nil {
			logger.Error().Msg("Failed to get versions")
			return fmt.Errorf("get versions failed: %w", err)
		}

		var jobs []engine.Job
		for _, version := range versions {
			versionNode, err := tree.GetNodeForPath(r.node, version.Path)
			if err != nil {
				logger.Error().Msg("Failed to get node for version")
				return fmt.Errorf("get version failed: %w", err)
			}
			job := NewVersionJob(r.srcAdapter, r.destAdapter, r.srcRegistry, r.destRegistry, r.artifactType, r.pkg,
				version,
				versionNode, r.stats)
			jobs = append(jobs, job)
		}

		log.Info().Msgf("Jobs length: %d", len(jobs))

		eng := engine.NewEngine(5, jobs)
		err = eng.Execute(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Engine execution failed")
			return fmt.Errorf("engine execution failed: %w", err)
		}
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed package migration step")
	return nil
}

// Post Any post processing work
func (r *Package) Post(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "post").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting package post-migration step")

	startTime := time.Now()
	// Your post-migration code here

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed registry post-migration step")
	return nil
}
