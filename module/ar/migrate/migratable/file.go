package migratable

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"harness/module/ar/migrate/adapter"
	"harness/module/ar/migrate/engine"
	"harness/module/ar/migrate/types"
	"time"
)

type File struct {
	srcRegistry  string
	destRegistry string
	srcAdapter   adapter.Adapter
	destAdapter  adapter.Adapter
	artifactType types.ArtifactType
	logger       zerolog.Logger
	pkg          types.Package
	version      types.Version
	file         *types.File
	node         *types.TreeNode
}

func NewFileJob(
	src adapter.Adapter,
	dest adapter.Adapter,
	srcRegistry string,
	destRegistry string,
	artifactType types.ArtifactType,
	pkg types.Package,
	version types.Version,
	node *types.TreeNode,
	file *types.File,
) engine.Job {
	jobID := uuid.New().String()

	jobLogger := log.With().
		Str("job_type", "file").
		Str("job_id", jobID).
		Str("source_registry", srcRegistry).
		Str("dest_registry", destRegistry).
		Str("package", pkg.Name).
		Str("version", version.Name).
		Str("file", file.Uri).
		Logger()

	return &File{
		srcRegistry:  srcRegistry,
		destRegistry: destRegistry,
		srcAdapter:   src,
		destAdapter:  dest,
		artifactType: artifactType,
		logger:       jobLogger,
		pkg:          pkg,
		node:         node,
		version:      version,
		file:         file,
	}
}

func (r File) Info() string {
	return r.srcRegistry + ":" + r.destRegistry + ":" + r.file.Uri
}

func (r File) Pre(ctx context.Context) error {
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
		Msg("Completed file pre-migration step")
	return nil
}

func (r File) Migrate(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "migrate").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting file migration step")
	startTime := time.Now()

	if r.artifactType == types.GENERIC || r.artifactType == types.MAVEN {
		downloadFile, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.file.Uri)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to download file")
			return fmt.Errorf("download file failed: %w", err)
		}
		err = r.destAdapter.UploadFile(r.destRegistry, downloadFile, r.file, header, r.pkg.Name, r.version.Name,
			r.artifactType)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to upload file")
		}
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed version migration step")
	return nil
}

func (r File) Post(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "post").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting file post-migration step")

	startTime := time.Now()
	// Your post-migration code here

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed file post-migration step")
	return nil
}
