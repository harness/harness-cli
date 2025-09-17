package migratable

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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
	config       *types.Config
	registry     types.RegistryInfo
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
	config *types.Config,
	registry types.RegistryInfo,
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
		config:       config,
		registry:     registry,
	}
}

func (r *Version) Info() string {
	return r.srcRegistry + " " + r.pkg.Name + " " + r.version.Name
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
				r.version, r.node, file, r.stats, r.mapping, r.config, r.registry)
			jobs = append(jobs, job)
		}
	}
	if r.artifactType == types.GO {
		// 1. get all files .mod, .zip, .info
		// 2. download all files
		// 3. pass it to create version
		files, err := tree.GetAllFiles(r.node)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get files from tree")
			return fmt.Errorf("get files from tree failed: %w", err)
		}
		versionFiles := []*types.File{}
		for _, file := range files {
			if file.Folder {
				continue
			}
			extension := filepath.Ext(file.Name)
			if extension != ".zip" && extension != ".mod" && extension != ".info" {
				continue
			}
			fileVersion := strings.TrimSuffix(file.Name, extension)
			if fileVersion == r.version.Name {
				versionFiles = append(versionFiles, file)
			}
		}
		downloadedFiles := []*types.PackageFiles{}
		for _, file := range versionFiles {
			downloadFile, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, file.Uri)
			if err != nil {
				logger.Error().Err(err).Msgf("Failed to download file %s", file.Name)
				return fmt.Errorf("download file %s failed: %w", file.Name, err)
			}
			downloadedFiles = append(downloadedFiles, &types.PackageFiles{
				File:         file,
				DownloadFile: downloadFile,
				Header:       &header,
			})
		}

		err = r.destAdapter.CreateVersion(r.destRegistry, r.pkg.Name, r.version.Name, r.artifactType, downloadedFiles,
			nil)

		if err != nil {
			return err
		}
	}

	if r.artifactType == types.DOCKER || r.artifactType == types.HELM || r.artifactType == types.HELM_LEGACY {
		log.Error().Ctx(ctx).Msgf("OCI migrate version is not supported")
		return fmt.Errorf("OCI migrate version is not supported")
	}

	log.Info().Msgf("Jobs length: %d", len(jobs))

	eng := engine.NewEngine(r.config.Concurrency, jobs)
	err := eng.Execute(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Engine execution saw following errors")
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
