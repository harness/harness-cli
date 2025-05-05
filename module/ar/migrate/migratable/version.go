package migratable

import (
	"context"
	"fmt"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/engine"
	"harness/module/ar/migrate/tree"
	"harness/module/ar/migrate/types"
	"time"

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
		Msg("Completed registry pre-migration step")
	return nil
}

func (r *Version) Migrate(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "migrate").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting registry migration step")
	startTime := time.Now()

	if r.artifactType == types.GENERIC {
		files, err := tree.GetAllFiles(r.node)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get files from tree")
			return fmt.Errorf("get files from tree failed: %w", err)
		}
		for _, file := range files {
			downloadFile, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, file.Uri)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to download file")
				return fmt.Errorf("download file failed: %w", err)
			}
			err = r.destAdapter.UploadFile(r.destRegistry, downloadFile, file, header, r.pkg.Name, r.version.Name)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to upload file")
				//return fmt.Errorf("upload file failed: %w", err)
			}
		}
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
		Msg("Completed registry post-migration step")
	return nil
}
