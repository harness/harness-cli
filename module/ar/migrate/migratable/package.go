package migratable

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/uuid"
	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/lib"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"helm.sh/helm/v3/pkg/repo"
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
			stat.Status = types.StatusFail
		} else {
			pterm.Success.Println(fmt.Sprintf("Copy repository %s to %s completed", srcImage, dstImage))
		}
		r.stats.FileStats = append(r.stats.FileStats, stat)

	} else if r.artifactType == types.HELM_LEGACY {
		r.migrateLegacyHelm(ctx)
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

func (r *Package) migrateLegacyHelm(ctx context.Context) error {
	file, _, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.URL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download helm chart %s", r.pkg.URL)
		pterm.Error.Println(fmt.Sprintf("Failed to download helm chart %s", r.pkg.URL))
		return err
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "*.tgz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	_, err = io.Copy(tmp, file)
	if err != nil {
		return err
	}

	// Ensure the file is closed and flushed to disk
	if err := tmp.Close(); err != nil {
		return err
	}

	// Create the reference for the destination chart
	refStr := fmt.Sprintf("%s/%s:%s", r.destRegistry, r.pkg.Name, r.pkg.Version)

	// Set up the keychain for authentication
	kc := lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.srcRegistry, r.destRegistry)
	// Register the keychain as the default for use by crane
	crane.WithAuth(kc)

	// Push the chart to destination
	pterm.Info.Println(fmt.Sprintf("Pushing helm chart %s to %s", r.pkg.Name, refStr))
	err = pushChart(tmp.Name(), refStr)

	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     0,
		Status:   types.StatusSuccess,
	}

	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to push helm chart %s to %s", r.pkg.Name, refStr)
		pterm.Error.Println(fmt.Sprintf("Failed to push helm chart %s to %s", r.pkg.Name, refStr))
		stat.Error = err.Error()
		stat.Status = types.StatusFail
		r.stats.FileStats = append(r.stats.FileStats, stat)
		return err
	}

	pterm.Success.Println(fmt.Sprintf("Successfully pushed helm chart %s to %s", r.pkg.Name, refStr))
	r.stats.FileStats = append(r.stats.FileStats, stat)
	return nil
}

// pushChart uploads chart.tar.gz --> oci://<dstRef>
func pushChart(tgzPath, dstRef string, ropts ...remote.Option) error {
	ref, err := name.ParseReference(dstRef)
	if err != nil {
		return err
	}

	chartData, err := os.ReadFile(tgzPath)
	if err != nil {
		return fmt.Errorf("read chart file: %w", err)
	}

	layer, err := static.NewLayer(chartData,
		types.MediaType("application/vnd.cncf.helm.chart.layer.v1.tar+gzip"))
	if err != nil {
		return fmt.Errorf("create layer: %w", err)
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return fmt.Errorf("append layer: %w", err)
	}

	annotations := map[string]string{
		"org.opencontainers.image.title":       filepath.Base(tgzPath),
		"org.opencontainers.image.created":     time.Now().Format(time.RFC3339),
		"org.opencontainers.image.description": "Helm chart migrated by Harness CLI",
		"org.opencontainers.artifactType":      "application/vnd.cncf.helm.chart.layer.v1.tar+gzip",
	}
	img = mutate.Annotations(img, annotations)
	if err != nil {
		return fmt.Errorf("set annotations: %w", err)
	}

	fmt.Printf("  → pushing %s …\n", ref.String())
	return remote.Write(ref, img, ropts...)
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
