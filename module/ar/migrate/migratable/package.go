package migratable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/lib"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/uuid"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

type Package struct {
	srcRegistry   string
	destRegistry  string
	srcAdapter    adapter.Adapter
	destAdapter   adapter.Adapter
	artifactType  types.ArtifactType
	logger        zerolog.Logger
	pkg           types.Package
	node          *types.TreeNode
	stats         *types.TransferStats
	skipMigration bool
	mapping       *types.RegistryMapping
	concurrency   int
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
	mapping *types.RegistryMapping,
	concurrency int,
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
		mapping:      mapping,
		concurrency:  concurrency,
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

	if r.artifactType == types.HELM_LEGACY && r.pkg.Name != "" && r.pkg.Version != "" && r.mapping.Ref != "" {
		exists, err := r.destAdapter.VersionExists(ctx,
			util.GetRegistryRef(config.Global.AccountID, r.mapping.Ref, r.destRegistry), r.pkg.Name, r.pkg.Version,
			r.artifactType)
		if err != nil {
			log.Error().Err(err).Msg("Failed to check if version exists")
			return nil
		}
		if exists {
			util.GetSkipPrinter().Println(fmt.Sprintf("Registry [%s], Package [%s/%s] already exists", r.destRegistry,
				r.pkg.Name, r.pkg.Version))
			r.skipMigration = true
			stat := types.FileStat{
				Name:     r.pkg.Name,
				Registry: r.srcRegistry,
				Uri:      r.pkg.Version,
				Size:     int64(r.pkg.Size),
				Status:   types.StatusSkip,
			}
			r.stats.FileStats = append(r.stats.FileStats, stat)
			return nil
		}
	}

	if (r.artifactType == types.DOCKER || r.artifactType == types.HELM) && (r.mapping.Ref != "") {
		srcImage, _ := r.srcAdapter.GetOCIImagePath(r.srcRegistry, r.pkg.Name)
		dstImage, _ := r.destAdapter.GetOCIImagePath(r.destRegistry, r.pkg.Name)
		logger.Info().Ctx(ctx).Msgf("Checking if should be skipped -- repository %s to %s", srcImage, dstImage)

		craneOpts := []crane.Option{
			crane.WithContext(ctx),
			crane.WithJobs(r.concurrency),
			crane.WithNoClobber(true),
			crane.WithAuthFromKeychain(lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.srcRegistry,
				r.destRegistry)),
		}
		if r.srcAdapter.GetConfig().Insecure {
			craneOpts = append(craneOpts, crane.Insecure)
		}

		tags, err := crane.ListTags(srcImage, craneOpts...)
		co := crane.GetOptions(craneOpts...)
		remoteOpts := co.Remote
		if err != nil {
			return err
		}
		for _, tag := range tags {
			dst := fmt.Sprintf("%s:%s", dstImage, tag)
			// HEAD the destination tag – 200 ⇒ already present.
			reference, _ := name.ParseReference(dst)
			if _, err := remote.Head(reference, remoteOpts...); err == nil {
				util.GetSkipPrinter().Println(fmt.Sprintf("Registry [%s], Package [%s:%s] already exists",
					r.destRegistry,
					r.pkg.Name, tag))
				stat := types.FileStat{
					Name:     r.pkg.Name,
					Registry: r.srcRegistry,
					Uri:      r.pkg.Name + ":" + tag,
					Size:     0,
					Status:   types.StatusSkip,
				}
				r.stats.FileStats = append(r.stats.FileStats, stat)
			}
		}

	}

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

	if r.skipMigration {
		logger.Info().Msg("Skipping migration as version already exists in destination registry")
		return nil
	}

	if r.artifactType == types.DOCKER || r.artifactType == types.HELM {
		srcImage, _ := r.srcAdapter.GetOCIImagePath(r.srcRegistry, r.pkg.Name)
		dstImage, _ := r.destAdapter.GetOCIImagePath(r.destRegistry, r.pkg.Name)
		pterm.Info.Println(fmt.Sprintf("Copying repository %s to %s", srcImage, dstImage))
		logger.Info().Ctx(ctx).Msgf("Copying repository %s to %s", srcImage, dstImage)

		craneOpts := []crane.Option{
			crane.WithUserAgent("harness-cli"),
			crane.WithContext(ctx),
			crane.WithJobs(r.concurrency),
			crane.WithNoClobber(true),
			crane.WithAuthFromKeychain(lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.srcRegistry,
				r.destRegistry)),
		}

		if r.srcAdapter.GetConfig().Insecure {
			craneOpts = append(craneOpts, crane.Insecure)
		}

		err := crane.CopyRepository(
			srcImage,
			dstImage,
			craneOpts...,
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
		// TODO: Replace by providing function to this migration job instead of complete implementation here.
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
				version, versionNode, r.stats, r.mapping, r.concurrency)
			jobs = append(jobs, job)
		}

		log.Info().Msgf("Jobs length: %d", len(jobs))

		eng := engine.NewEngine(r.concurrency, jobs)
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

	if err := tmp.Close(); err != nil {
		return err
	}

	refStr, err := r.destAdapter.GetOCIImagePath(r.destRegistry, r.pkg.Name)
	if err != nil {
		return err
	}
	refStr += ":" + r.pkg.Version
	pterm.Info.Println(fmt.Sprintf("Pushing helm chart %s to %s", r.pkg.Name, refStr))
	err = r.pushChart(ctx, tmp.Name(), refStr)

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

const labelMaxBytes = 1024

func readChartMeta(path string) (*chart.Metadata, error) {
	ch, err := loader.Load(path) // understands .tgz & directories
	if err != nil {
		return nil, err
	}
	return ch.Metadata, nil
}

func truncate(s string) string {
	_max := labelMaxBytes
	if len(s) <= _max {
		return s
	}
	// walk backwards until we’re on a rune boundary
	for _max > 0 && !utf8.RuneStart(s[_max]) {
		_max--
	}
	return s[:_max-1] + "…"
}

func chartLabels(meta *chart.Metadata) map[string]string {
	lbl := map[string]string{
		"helm.sh/chart":     truncate(meta.Name + "-" + meta.Version),
		"chart.name":        truncate(meta.Name),
		"chart.home":        truncate(meta.Home),
		"chart.sources":     truncate(strings.Join(meta.Sources, ",")),
		"chart.version":     truncate(meta.Version),
		"chart.description": truncate(meta.Description),
		"chart.keywords":    truncate(strings.Join(meta.Keywords, ",")),
		"chart.icon":        truncate(meta.Icon),
		"chart.apiVersion":  truncate(meta.APIVersion),
		"chart.condition":   truncate(meta.Condition),
		"chart.tags":        truncate(meta.Tags),
		"chart.appVersion":  truncate(meta.AppVersion),
		"chart.kubeVersion": truncate(meta.KubeVersion),
		"chart.type":        truncate(meta.Type),
	}
	// objects & complex lists → JSON
	if b, _ := json.Marshal(meta.Maintainers); len(b) > 0 {
		lbl["chart.maintainers"] = truncate(string(b))
	}
	if b, _ := json.Marshal(meta.Dependencies); len(b) > 0 {
		lbl["chart.dependencies"] = truncate(string(b))
	}
	if b, _ := json.Marshal(meta.Annotations); len(b) > 0 {
		lbl["chart.annotations"] = truncate(string(b))
	}
	return lbl
}

// pushChart uploads chart.tar.gz --> oci://<dstRef>
func (r *Package) pushChart(ctx context.Context, chartPath string, dstRef string) error {
	meta, _ := readChartMeta(chartPath)
	labels := chartLabels(meta)
	ref, err := name.ParseReference(dstRef, name.WeakValidation)
	//check(err, "parsing reference")

	chartData, err := os.ReadFile(chartPath)
	check(err, "reading chart file")

	layer := static.NewLayer(chartData, "application/vnd.cncf.helm.chart.content.v1.tar+gzip")

	img, err := mutate.AppendLayers(empty.Image, layer)
	check(err, "appending layer")

	cfg := v1.Config{
		Labels: labels,
	}
	cfg.Labels = map[string]string{}

	img, err = mutate.Config(img, cfg)

	check(err, "adding config JSON")

	img = mutate.ConfigMediaType(img, "application/vnd.cncf.helm.config.v1+json")
	annotations := map[string]string{
		"org.opencontainers.image.title":       truncate(meta.Name),
		"org.opencontainers.image.description": truncate(meta.Description),
		"org.opencontainers.image.version":     truncate(meta.Version),
		"org.opencontainers.image.created":     time.Now().UTC().Format(time.RFC3339),
		"org.opencontainers.artifactType":      "application/vnd.cncf.helm.chart.layer.v1.tar+gzip",
	}

	if strings.Contains(meta.Version, "+") {
		return errors.New("chart version cannot contain +")
	}

	for k, v := range meta.Annotations {
		annotations[k] = truncate(v)
	}

	img = mutate.Annotations(img, annotations).(v1.Image)

	craneOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithUserAgent("harness-cli"),
		remote.WithAuthFromKeychain(lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.srcRegistry,
			r.destRegistry)),
	}

	err = remote.Write(ref, img,
		craneOpts...,
	)
	return err
}

func check(err error, context string) {
	if err != nil {
		log.Error().Msgf("❌  %s: %v", context, err)
	}
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
