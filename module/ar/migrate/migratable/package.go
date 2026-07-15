package migratable

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/lib"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/harness/harness-cli/util/artifact"
	"github.com/harness/harness-cli/util/common"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	types2 "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/uuid"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

type Package struct {
	srcRegistry           string
	sourcePackageHostname string
	destRegistry          string
	srcAdapter            adapter.Adapter
	destAdapter           adapter.Adapter
	artifactType          types.ArtifactType
	logger                zerolog.Logger
	pkg                   types.Package
	node                  *types.TreeNode
	stats                 *types.TransferStats
	skipMigration         bool
	mapping               *types.RegistryMapping
	config                *types.Config
	registry              types.RegistryInfo
	dryRunStats           *types.DryRunStats
}

func NewPackageJob(
	src adapter.Adapter,
	dest adapter.Adapter,
	srcRegistry string,
	sourcePackageHostname string,
	destRegistry string,
	artifactType types.ArtifactType,
	pkg types.Package,
	node *types.TreeNode,
	stats *types.TransferStats,
	mapping *types.RegistryMapping,
	config *types.Config,
	registry types.RegistryInfo,
	dryRunStats *types.DryRunStats,
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
		srcRegistry:           srcRegistry,
		sourcePackageHostname: sourcePackageHostname,
		destRegistry:          destRegistry,
		srcAdapter:            src,
		destAdapter:           dest,
		artifactType:          artifactType,
		logger:                jobLogger,
		pkg:                   pkg,
		node:                  node,
		stats:                 stats,
		mapping:               mapping,
		config:                config,
		registry:              registry,
		dryRunStats:           dryRunStats,
	}
}

func (r *Package) Info() string {
	return r.srcRegistry + " " + r.pkg.Name + " " + r.pkg.Version
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

	// Skip destination checks in dry-run mode
	if r.config.DryRun {
		logger.Info().Msg("Dry-run mode: skipping destination package checks")
		logger.Info().
			Dur("duration", time.Since(startTime)).
			Msg("Completed package pre-migration step (dry-run)")
		return nil
	}

	if !r.config.Overwrite && ((r.artifactType == types.HELM_LEGACY || r.artifactType == types.HELM_HTTP) && r.pkg.Name != "" && r.pkg.Version != "") {
		exists, err := r.destAdapter.VersionExists(ctx, r.pkg,
			r.registry.Path, r.pkg.Name, r.pkg.Version,
			r.artifactType)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to check if version exists Registry [%s], Package [%s/%s]",
				r.destRegistry,
				r.pkg.Name, r.pkg.Version)
			return nil
		}
		if exists {
			util.GetSkipPrinter().Println(fmt.Sprintf("Registry [%s], Package [%s/%s] already exists", r.destRegistry,
				r.pkg.Name, r.pkg.Version))
			r.skipMigration = true
			stat := types.FileStat{
				Name:     r.pkg.Name,
				Registry: r.srcRegistry,
				Uri:      r.pkg.URL,
				Size:     int64(r.pkg.Size),
				Status:   types.StatusSkip,
			}
			r.stats.FileStats = append(r.stats.FileStats, stat)
			return nil
		}
	}

	if !r.config.Overwrite && (r.artifactType == types.DOCKER || r.artifactType == types.HELM) {
		srcImage, _ := r.srcAdapter.GetOCIImagePath(r.srcRegistry, r.sourcePackageHostname, r.pkg.Name)
		dstImage, _ := r.destAdapter.GetOCIImagePath(r.destRegistry, "", r.pkg.Name)
		logger.Info().Ctx(ctx).Msgf("Checking if should be skipped -- repository %s to %s", srcImage, dstImage)

		keyChain, err := lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.sourcePackageHostname)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("Failed to create keyChain: %v", err)
			pterm.Error.Println(fmt.Sprintf("Failed to create keyChain: %v", err))
			return err
		}

		craneOpts := []crane.Option{
			crane.WithContext(ctx),
			crane.WithJobs(r.config.Concurrency),
			crane.WithNoClobber(!r.config.Overwrite),
			crane.WithAuthFromKeychain(keyChain),
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
			logger.Info().Ctx(ctx).Msgf("Checking if dst %s already exists", dst)
			reference, err := name.ParseReference(dst)
			if err != nil {
				logger.Error().Err(err).Str("dst", dst).Msgf("Failed to parse destination reference %q, skipping tag",
					dst)
				continue
			}
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

	// In dry-run mode, add package to directory structure
	if r.config.DryRun && r.dryRunStats != nil {
		r.addPackageToDryRunDirectory()
		logger.Info().Msgf("Dry-run: processing package %s", r.pkg.Name)
	}

	if r.artifactType == types.DOCKER || r.artifactType == types.HELM {
		if r.config.DryRun {
			logger.Info().Msgf("Dry-run: would copy repository %s/%s to %s", r.srcRegistry, r.pkg.Name, r.destRegistry)
			return nil
		}

		r.migrateOCI(ctx, logger)

	} else if r.artifactType == types.HELM_LEGACY {
		// TODO: Replace by providing function to this migration job instead of complete implementation here.
		r.migrateLegacyHelm(ctx)
	} else if r.artifactType == types.HELM_HTTP {
		r.migrateHelmHTTP(ctx)
	} else if r.artifactType == types.RPM {
		r.migrateRPM(ctx)
	} else if r.artifactType == types.DEBIAN {
		r.migrateDebian(ctx)
	} else if r.artifactType == types.CONDA {
		r.migrateConda(ctx)
	} else if r.artifactType == types.COMPOSER {
		r.migrateComposer(ctx)
	} else if r.artifactType == types.SWIFT {
		r.migrateSwift(ctx)
	} else if r.artifactType == types.CONAN {
		r.migrateConan(ctx)
	} else {
		versions, err := r.srcAdapter.GetVersions(r.pkg, r.node, r.srcRegistry, r.pkg.Name, r.artifactType)
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
				version, versionNode, r.stats, r.mapping, r.config, r.registry, r.dryRunStats)
			jobs = append(jobs, job)
		}

		log.Info().Msgf("Jobs length: %d", len(jobs))

		eng := engine.NewEngine(r.config.Concurrency, jobs)
		err = eng.Execute(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Engine execution saw following errors")
		}
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed package migration step")
	return nil
}

// migrateOCI copies a Docker/Helm-OCI image repository from source to
// destination.
//
// When Overwrite is true, every tag is pushed unconditionally, so the fast
// path is crane.CopyRepository, which copies every tag in parallel and is the
// well-tested common case. Its weakness is all-or-nothing: it runs every tag
// inside a single errgroup, so the first tag whose manifest cannot be fetched
// (e.g. an orphaned tag whose manifest was garbage-collected at the source —
// the registry answers MANIFEST_UNKNOWN) cancels the shared context and
// aborts every remaining tag, marking the whole image as failed. If the bulk
// copy fails, we fall back to copyTagsIndividually.
//
// When Overwrite is false, we skip the bulk fast path entirely: its no-clobber
// check only tests whether a destination tag NAME exists, so it can never
// detect a tag that was re-pointed to a different digest at the source (e.g.
// a "latest" tag moved to a new image) — that tag would be skipped and go
// stale at the destination forever. Instead we go straight to
// copyTagsIndividually, which compares source/destination digests per tag and
// pushes only when the tag is missing or its digest differs, so a moved tag
// is corrected without needing Overwrite to be true.
//
// Either way, the image contributes exactly ONE stat: Success when at least
// one tag migrated or every tag was already in sync/present, Fail on a
// genuine per-tag failure, and Skip when there was nothing to do.
func (r *Package) migrateOCI(ctx context.Context, logger zerolog.Logger) {
	srcImage, _ := r.srcAdapter.GetOCIImagePath(r.srcRegistry, r.sourcePackageHostname, r.pkg.Name)
	dstImage, _ := r.destAdapter.GetOCIImagePath(r.destRegistry, "", r.pkg.Name)

	pterm.Info.Println(fmt.Sprintf("Copying repository %s to %s", srcImage, dstImage))
	logger.Info().Ctx(ctx).Msgf("Copying repository %s to %s", srcImage, dstImage)

	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      srcImage,
		Size:     0,
		Status:   types.StatusSuccess,
	}

	keyChain, err := lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.sourcePackageHostname)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to create keyChain: %v", err)
		pterm.Error.Println(fmt.Sprintf("Failed to create keyChain: %v", err))
		stat.Error = err.Error()
		stat.Status = types.StatusFail
		r.stats.FileStats = append(r.stats.FileStats, stat)
		return
	}

	craneOpts := []crane.Option{
		crane.WithUserAgent("harness-cli"),
		crane.WithContext(ctx),
		crane.WithJobs(r.config.Concurrency),
		crane.WithNoClobber(!r.config.Overwrite),
		crane.WithAuthFromKeychain(keyChain),
	}
	if r.srcAdapter.GetConfig().Insecure {
		craneOpts = append(craneOpts, crane.Insecure)
	}

	if !r.config.Overwrite {
		res, tagErr := r.copyTagsIndividually(ctx, logger, srcImage, dstImage, craneOpts)
		r.finishOCICopy(ctx, &stat, res, tagErr, srcImage, dstImage)
		r.stats.FileStats = append(r.stats.FileStats, stat)
		return
	}

	// Fast path: bulk parallel copy of every tag.
	if err := crane.CopyRepository(srcImage, dstImage, craneOpts...); err == nil {
		pterm.Success.Println(fmt.Sprintf("Copy repository %s to %s completed", srcImage, dstImage))
		r.stats.FileStats = append(r.stats.FileStats, stat)
		return
	}
	log.Warn().Ctx(ctx).
		Msgf("Bulk copy of %s failed; retrying tag-by-tag to isolate stale/orphaned tags", srcImage)
	pterm.Warning.Println(fmt.Sprintf("Bulk copy of %s failed; retrying tag-by-tag", srcImage))

	// Slow path: a bad tag took down the bulk copy. Retry per tag so orphaned
	// source manifests are skipped and the rest of the image still migrates.
	res, tagErr := r.copyTagsIndividually(ctx, logger, srcImage, dstImage, craneOpts)
	r.finishOCICopy(ctx, &stat, res, tagErr, srcImage, dstImage)
	r.stats.FileStats = append(r.stats.FileStats, stat)
}

// finishOCICopy sets stat's Status/Error from a copyTagsIndividually result,
// preserving the single-stat-per-image contract described on migrateOCI:
// Success when at least one tag migrated or all tags were already in
// sync/skipped, Skip when there was nothing to do at all, Fail on a genuine
// per-tag failure.
func (r *Package) finishOCICopy(
	ctx context.Context, stat *types.FileStat, res copyResult, err error, srcImage, dstImage string,
) {
	switch {
	case err != nil:
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to copy repository %s to %s", srcImage, dstImage)
		pterm.Error.Println(fmt.Sprintf("Failed to copy repository %s to %s", srcImage, dstImage))
		stat.Error = err.Error()
		stat.Status = types.StatusFail
	case res.migrated == 0 && res.skipped == 0:
		// No tags at all: nothing was copied and nothing was pre-existing.
		// Recording Success here would mask a source that resolved to an empty
		// repository, so mark it Skip instead.
		stat.Status = types.StatusSkip
		pterm.Warning.Println(fmt.Sprintf("Repository %s had no tags to copy", srcImage))
	case res.migrated == 0:
		// Every tag was already in sync (or an orphaned source we skipped); the
		// image is effectively up to date, so this is a success.
		pterm.Success.Println(fmt.Sprintf(
			"Copy repository %s to %s completed (all %d tags already in sync/skipped)", srcImage, dstImage, res.skipped))
	default:
		pterm.Success.Println(fmt.Sprintf("Copy repository %s to %s completed", srcImage, dstImage))
	}
}

// copyResult summarises the outcome of a per-tag copy pass so the caller can
// choose a single honest image-level stat.
type copyResult struct {
	migrated int
	skipped  int
	failed   int
	total    int
}

// copyTagsIndividually copies each tag of an image independently so one bad tag
// cannot abort the rest, and so a tag whose source digest has changed (e.g. a
// tag re-pointed to a different image) is still corrected at the destination.
//
// The tags are copied in parallel (bounded by the configured concurrency), but
// — unlike the bulk path — a single tag's failure never cancels its siblings.
// For each tag, the source and destination digests are compared (via a cheap
// HEAD, crane.Digest) rather than relying on crane's name-based no-clobber:
//   - destination tag missing, or its digest differs from the source → push
//     (this is what corrects a moved tag without needing Overwrite).
//   - destination digest already matches the source → skip, no push needed.
//   - a tag whose SOURCE manifest is missing/orphaned is skipped.
//   - any other error is a genuine failure.
//
// It returns a summary plus a non-nil error iff at least one tag genuinely
// failed, so the caller can set a single image-level stat — this function does
// NOT touch r.stats.FileStats itself.
func (r *Package) copyTagsIndividually(
	ctx context.Context, logger zerolog.Logger, srcImage, dstImage string, craneOpts []crane.Option,
) (copyResult, error) {
	var res copyResult

	// Resolve the SOURCE registry host once. crane.Copy conflates source-fetch
	// and destination-push errors into one *transport.Error, so we key the
	// stale-manifest classifier on the failing request's host: only a not-found
	// against the SOURCE is a stale/orphaned manifest we may skip.
	srcHost := ""
	if srcRepo, perr := name.NewRepository(srcImage, crane.GetOptions(craneOpts...).Name...); perr == nil {
		srcHost = srcRepo.RegistryStr()
	}

	// Enumerate source tags. A failure here is a genuine image-level failure
	// (auth, DNS, repository gone) — there is nothing to iterate.
	tags, err := crane.ListTags(srcImage, craneOpts...)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to list tags for %s", srcImage)
		return res, fmt.Errorf("list source tags for %s: %w", srcImage, err)
	}
	res.total = len(tags)

	// Push must be able to overwrite a destination tag whose digest differs
	// from the source, so no-clobber is dropped here — the digest comparison
	// below is what decides whether a push happens, replacing name-based
	// no-clobber as the skip mechanism.
	pushOpts := withoutNoClobber(craneOpts)

	var (
		mu         sync.Mutex
		failedErrs []error
	)
	g, _ := errgroup.WithContext(ctx)
	if r.config.Concurrency > 0 {
		g.SetLimit(r.config.Concurrency)
	}

	for _, tag := range tags {
		g.Go(func() error {
			src := fmt.Sprintf("%s:%s", srcImage, tag)
			dst := fmt.Sprintf("%s:%s", dstImage, tag)

			srcDigest, digestErr := crane.Digest(src, craneOpts...)
			if digestErr != nil {
				mu.Lock()
				defer mu.Unlock()
				if isStaleSourceManifestErr(digestErr, srcHost) {
					res.skipped++
					logger.Warn().Ctx(ctx).Err(digestErr).
						Msgf("Skipping tag %s: source manifest missing/orphaned", src)
					pterm.Warning.Println(fmt.Sprintf("Skipping %s: source manifest missing/orphaned (%v)", src, digestErr))
				} else {
					failedErrs = append(failedErrs, fmt.Errorf("%s: resolve source digest: %w", tag, digestErr))
					logger.Error().Ctx(ctx).Err(digestErr).Msgf("Failed to resolve source digest for %s", src)
					pterm.Error.Println(fmt.Sprintf("Failed to resolve source digest for %s", src))
				}
				return nil
			}

			dstDigest, dstErr := crane.Digest(dst, craneOpts...)
			if dstErr == nil && dstDigest == srcDigest {
				mu.Lock()
				res.skipped++
				logger.Info().Ctx(ctx).Msgf("Skipping %s: destination already in sync (%s)", dst, dstDigest)
				mu.Unlock()
				return nil
			}

			// Destination tag missing, or present with a different digest (e.g.
			// the tag was moved to a new image at the source) — push to bring it
			// in sync.
			copyErr := crane.Copy(src, dst, pushOpts...)

			mu.Lock()
			defer mu.Unlock()
			switch {
			case copyErr == nil:
				res.migrated++
				pterm.Success.Println(fmt.Sprintf("Copied %s to %s", src, dst))
			case isStaleSourceManifestErr(copyErr, srcHost):
				// Orphaned/stale SOURCE manifest — the registry tag references a
				// manifest digest that no longer exists at the source. Skip this
				// tag and keep migrating the rest of the image.
				res.skipped++
				logger.Warn().Ctx(ctx).Err(copyErr).
					Msgf("Skipping tag %s: source manifest missing/orphaned", src)
				pterm.Warning.Println(fmt.Sprintf("Skipping %s: source manifest missing/orphaned (%v)", src, copyErr))
			default:
				failedErrs = append(failedErrs, fmt.Errorf("%s: %w", tag, copyErr))
				logger.Error().Ctx(ctx).Err(copyErr).Msgf("Failed to copy tag %s to %s", src, dst)
				pterm.Error.Println(fmt.Sprintf("Failed to copy %s to %s", src, dst))
			}
			// Never return the error: a per-tag failure must not cancel the
			// group and abort the remaining tags — isolation is the whole point.
			return nil
		})
	}
	_ = g.Wait()
	res.failed = len(failedErrs)

	logger.Info().Ctx(ctx).
		Int("migrated", res.migrated).
		Int("skipped", res.skipped).
		Int("failed", res.failed).
		Int("total_tags", res.total).
		Msgf("Completed per-tag OCI repository copy %s to %s", srcImage, dstImage)
	pterm.Info.Println(fmt.Sprintf(
		"Repository %s: %d migrated, %d skipped, %d failed (of %d tags)",
		r.pkg.Name, res.migrated, res.skipped, res.failed, res.total))

	if len(failedErrs) > 0 {
		return res, fmt.Errorf("%d of %d tags failed to copy: %w", len(failedErrs), res.total, errors.Join(failedErrs...))
	}
	return res, nil
}

// withoutNoClobber returns craneOpts with no-clobber forced off, appended
// last so it wins over any earlier WithNoClobber(true) in the slice. Used for
// the actual push once the digest comparison in copyTagsIndividually has
// already decided a push is needed — no-clobber must not then refuse it.
func withoutNoClobber(craneOpts []crane.Option) []crane.Option {
	out := make([]crane.Option, len(craneOpts), len(craneOpts)+1)
	copy(out, craneOpts)
	return append(out, crane.WithNoClobber(false))
}

// isStaleSourceManifestErr reports whether err indicates that the SOURCE
// manifest/blob a tag points at does not exist. These are the Docker registry
// v2 error codes returned when a referenced manifest/blob is gone, plus a bare
// 404 with no structured body. Such tags are safe to skip so the rest of the
// image can migrate.
//
// crane.Copy pulls from the source (including lazy child-manifest fetches while
// pushing a manifest list) and pushes to the destination, surfacing both as one
// *transport.Error. So a not-found-class error is only treated as a stale SOURCE
// manifest when the failing request targeted the source registry (srcHost); the
// same code from the DESTINATION push is a genuine failure and must NOT be
// swallowed. When the request host cannot be determined we fall back to the
// error code/status alone.
func isStaleSourceManifestErr(err error, srcHost string) bool {
	if err == nil {
		return false
	}
	var terr *transport.Error
	if !errors.As(err, &terr) {
		return false
	}
	if srcHost != "" && terr.Request != nil && terr.Request.URL != nil &&
		terr.Request.URL.Host != srcHost {
		// The failing request went to a registry other than the source (i.e. the
		// destination push) — not a stale source manifest.
		return false
	}
	for _, d := range terr.Errors {
		switch d.Code {
		case transport.ManifestUnknownErrorCode,
			transport.ManifestBlobUnknownErrorCode,
			transport.BlobUnknownErrorCode,
			transport.NameUnknownErrorCode:
			return true
		}
	}
	// A 404 with no machine-readable error body (some registries/proxies return
	// a bare Not Found) is treated as a missing source manifest too.
	if len(terr.Errors) == 0 && terr.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

func (r *Package) migrateLegacyHelm(ctx context.Context) error {
	if r.config.DryRun {
		log.Info().Ctx(ctx).Msgf("Dry-run: would migrate legacy helm chart %s", r.pkg.URL)
		return nil
	}
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

	refStr, err := r.destAdapter.GetOCIImagePath(r.destRegistry, r.sourcePackageHostname, r.pkg.Name)
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

// migrateHelmHTTP migrates a single Helm chart (and its optional .prov sidecar)
// from a source registry into a HAR HELM_HTTP registry.
//
// The flow is provider-agnostic: it only relies on srcAdapter.DownloadFile
// against pkg.URL (the chart) and pkg.URL+".prov" (the sidecar). The chart is
// streamed without a temp file (memory-safe for large charts); UploadFile owns
// closing the reader.
func (r *Package) migrateHelmHTTP(ctx context.Context) error {
	if r.config.DryRun {
		return nil
	}

	file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.URL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download helm chart %s", r.pkg.URL)
		pterm.Error.Println(fmt.Sprintf("Failed to download helm chart %s", r.pkg.URL))
		r.stats.FileStats = append(r.stats.FileStats, types.FileStat{
			Name:     r.pkg.Name,
			Registry: r.srcRegistry,
			Uri:      r.pkg.URL,
			Size:     int64(r.pkg.Size),
			Status:   types.StatusFail,
			Error:    err.Error(),
		})
		return err
	}

	// Canonical upload name: "<name>-<version>.tgz". pkg.Name may carry a nested
	// directory prefix (e.g. "ChartA/ChartB/abc") preserved verbatim so the
	// upload mirrors the source layout; the server strips the prefix and
	// validates the leaf against Chart.yaml.
	chartFile := util.GetChartFileName(r.pkg.Name, r.pkg.Version)

	title := fmt.Sprintf("%s (%s)", r.pkg.Name, common.GetSize(int64(r.pkg.Size)))
	pterm.Info.Println(fmt.Sprintf("Copying helm chart %s from %s to %s", chartFile, r.srcRegistry, r.destRegistry))
	err = r.destAdapter.UploadFile(
		r.destRegistry,
		file,
		&types.File{Name: chartFile, Uri: chartFile},
		header,
		r.pkg.Name,
		r.pkg.Version,
		types.RAW,
		nil,
	)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     int64(r.pkg.Size),
		Status:   types.StatusSuccess,
	}
	if err != nil {
		if errors.Is(err, types.ErrArtifactAlreadyExists) {
			stat.Status = types.StatusSkip
			pterm.Info.Println(fmt.Sprintf("%s already exists, skipping", title))
			r.stats.FileStats = append(r.stats.FileStats, stat)
			return nil
		}
		r.logger.Error().Err(err).Msg("Failed to upload helm chart")
		stat.Status = types.StatusFail
		stat.Error = err.Error()
		pterm.Error.Println(title)
		r.stats.FileStats = append(r.stats.FileStats, stat)
		// Do not attempt the provenance upload if the chart failed — the server
		// would reject a .prov with no chart (ErrChartNotFoundForProvenance).
		return err
	}
	pterm.Success.Println(title)
	r.stats.FileStats = append(r.stats.FileStats, stat)

	// Provenance is best-effort and only attempted after a successful chart
	// upload. A missing .prov is the normal case and must not be recorded as a
	// failure.
	r.migrateHelmHTTPProv(ctx)
	return nil
}

// migrateHelmHTTPProv migrates a chart's provenance sidecar if one exists. It is
// always called after the chart upload has succeeded. The sidecar is discovered
// as "<chartURL>.prov" in the source; if the source has no such file, the
// download fails and we simply skip (debug log only) — missing provenance is
// normal and is not a migration failure. When present, it is uploaded as
// "<name>-<version>.tgz.prov" so the server attaches it to the already-uploaded
// chart of the same identity.
func (r *Package) migrateHelmHTTPProv(ctx context.Context) {
	provURL := r.pkg.URL + ".prov"
	provFile, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, provURL)
	if err != nil {
		r.logger.Debug().Err(err).Msgf("No provenance file for chart %s (skipping)", r.pkg.URL)
		return
	}

	provBytes, err := io.ReadAll(provFile)
	_ = provFile.Close()
	if err != nil {
		r.logger.Debug().Err(err).Msgf("Could not read provenance file for chart %s (skipping)", r.pkg.URL)
		return
	}
	provSize := int64(len(provBytes))

	provName := util.GetChartProvFileName(r.pkg.Name, r.pkg.Version)
	pterm.Info.Println(fmt.Sprintf("Copying provenance %s from %s to %s", provName, r.srcRegistry, r.destRegistry))
	err = r.destAdapter.UploadFile(
		r.destRegistry,
		io.NopCloser(bytes.NewReader(provBytes)),
		&types.File{Name: provName, Uri: provName},
		header,
		r.pkg.Name,
		r.pkg.Version,
		types.RAW,
		nil,
	)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      provURL,
		Size:     provSize,
		Status:   types.StatusSuccess,
	}
	if err != nil {
		if errors.Is(err, types.ErrArtifactAlreadyExists) {
			stat.Status = types.StatusSkip
			pterm.Info.Println(fmt.Sprintf("Provenance %s already exists, skipping", provName))
		} else {
			r.logger.Error().Err(err).Msgf("Failed to upload provenance %s", provName)
			stat.Status = types.StatusFail
			stat.Error = err.Error()
			pterm.Error.Println(fmt.Sprintf("Failed to upload provenance %s", provName))
		}
	} else {
		pterm.Success.Println(fmt.Sprintf("Successfully uploaded provenance %s", provName))
	}
	r.stats.FileStats = append(r.stats.FileStats, stat)
}

func (r *Package) migrateConda(ctx context.Context) error {
	if r.config.DryRun {
		log.Info().Ctx(ctx).Msgf("Dry-run: would migrate conda package %s", r.pkg.Path)
		return nil
	}
	file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.Path)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download conda package %s", r.pkg.Path)
		pterm.Error.Println(fmt.Sprintf("Failed to download conda package %s", r.pkg.Path))
		return err
	}
	defer file.Close()

	metadata := make(map[string]interface{})
	metadata["X-File-Name"] = path.Base(r.pkg.Path)
	metadata["X-Subdir"] = strings.Split(r.pkg.Version, "/")[0]

	title := fmt.Sprintf("%s (%s)", r.pkg.Name, common.GetSize(int64(r.pkg.Size)))
	pterm.Info.Println(fmt.Sprintf("Copying file %s from %s to %s", r.pkg.Name, r.srcRegistry, r.destRegistry))
	err = r.destAdapter.UploadFile(
		r.destRegistry,
		file,
		&types.File{Uri: r.pkg.Path},
		header,
		r.pkg.Name,
		r.pkg.Version,
		r.artifactType,
		metadata,
	)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     int64(r.pkg.Size),
		Status:   types.StatusSuccess,
	}
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to upload file")
		stat.Status = types.StatusFail
		stat.Error = err.Error()
		pterm.Error.Println(title)
	} else {
		pterm.Success.Println(title)
	}
	r.stats.FileStats = append(r.stats.FileStats, stat)
	return nil
}

func (r *Package) migrateRPM(ctx context.Context) error {
	if r.config.DryRun {
		log.Info().Ctx(ctx).Msgf("Dry-run: would migrate RPM package %s", r.pkg.URL)
		return nil
	}
	file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.URL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download RPM package %s", r.pkg.URL)
		pterm.Error.Println(fmt.Sprintf("Failed to download RPM package %s", r.pkg.URL))
		return err
	}
	defer file.Close()

	title := fmt.Sprintf("%s (%s)", r.pkg.Name, common.GetSize(int64(r.pkg.Size)))
	pterm.Info.Println(fmt.Sprintf("Copying file %s from %s to %s", r.pkg.Name, r.srcRegistry, r.destRegistry))
	err = r.destAdapter.UploadFile(r.destRegistry, file, &types.File{Uri: r.pkg.URL}, header, r.pkg.Name, r.pkg.Name,
		r.artifactType, nil)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     int64(r.pkg.Size),
		Status:   types.StatusSuccess,
	}
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to upload file")
		stat.Status = types.StatusFail
		stat.Error = err.Error()
		pterm.Error.Println(title)
	} else {
		pterm.Success.Println(title)
	}
	r.stats.FileStats = append(r.stats.FileStats, stat)
	return nil
}

func (r *Package) migrateDebian(ctx context.Context) error {
	if r.config.DryRun {
		log.Info().Ctx(ctx).Msgf("Dry-run: would migrate Debian package %s", r.pkg.URL)
		return nil
	}
	file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.URL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download Debian package %s", r.pkg.URL)
		pterm.Error.Println(fmt.Sprintf("Failed to download Debian package %s", r.pkg.URL))
		return err
	}
	defer file.Close()

	title := fmt.Sprintf("%s (%s)", r.pkg.Name, common.GetSize(int64(r.pkg.Size)))
	pterm.Info.Println(fmt.Sprintf("Copying file %s from %s to %s", r.pkg.Name, r.srcRegistry, r.destRegistry))

	// Prepare metadata with distribution and component for Debian packages
	metadata := map[string]interface{}{
		"distribution": r.pkg.Metadata["distribution"],
		"component":    r.pkg.Metadata["component"],
	}

	// Determine file type
	// Debian packages can be either .deb (binary) or .dsc (source descriptor)
	// Source tar files are not processed as separate packages - they are uploaded
	// automatically when their parent .dsc file is processed (see below)
	isDscFile := strings.HasSuffix(r.pkg.Name, ".dsc")
	if isDscFile {
		metadata["fileType"] = "dsc"
		// Add source files list to metadata if available
		if sourceFiles, ok := r.pkg.Metadata["sourceFiles"]; ok && sourceFiles != "" {
			metadata["sourceFiles"] = sourceFiles
		}
	} else {
		// Assume .deb binary package
		metadata["fileType"] = "deb"
	}

	err = r.destAdapter.UploadFile(r.destRegistry, file, &types.File{Uri: r.pkg.URL}, header, r.pkg.Name, r.pkg.Name,
		r.artifactType, metadata)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     int64(r.pkg.Size),
		Status:   types.StatusSuccess,
	}
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to upload file")
		stat.Status = types.StatusFail
		stat.Error = err.Error()
		pterm.Error.Println(title)
	} else {
		pterm.Success.Println(title)
	}
	r.stats.FileStats = append(r.stats.FileStats, stat)

	// If this is a .dsc file and upload was successful, upload associated source files
	if isDscFile && err == nil {
		if sourceFilesStr, ok := r.pkg.Metadata["sourceFiles"]; ok && sourceFilesStr != "" {
			sourceFileNames := strings.Split(sourceFilesStr, ",")
			directory := r.pkg.Metadata["directory"]

			for _, srcFileName := range sourceFileNames {
				srcFileName = strings.TrimSpace(srcFileName)
				if srcFileName == "" {
					continue
				}

				// Construct the full path for the source file
				var srcFilePath string
				if directory != "" {
					srcFilePath = directory + "/" + srcFileName
				} else {
					srcFilePath = srcFileName
				}

				// Download the source file
				srcFile, srcHeader, err := r.srcAdapter.DownloadFile(r.srcRegistry, srcFilePath)
				if err != nil {
					log.Error().Ctx(ctx).Err(err).Msgf("Failed to download source file %s", srcFilePath)
					pterm.Error.Println(fmt.Sprintf("Failed to download source file %s", srcFilePath))
					r.stats.FileStats = append(r.stats.FileStats, types.FileStat{
						Name:     srcFileName,
						Registry: r.srcRegistry,
						Uri:      srcFilePath,
						Size:     0,
						Status:   types.StatusFail,
						Error:    err.Error(),
					})
					continue
				}

				srcTitle := fmt.Sprintf("%s (source file)", srcFileName)
				pterm.Info.Println(fmt.Sprintf("Copying source file %s from %s to %s", srcFileName, r.srcRegistry, r.destRegistry))

				// Prepare metadata for source file
				srcMetadata := map[string]interface{}{
					"distribution": r.pkg.Metadata["distribution"],
					"component":    r.pkg.Metadata["component"],
					"fileType":     "src",
					"package":      r.pkg.Metadata["packageName"],
				}

				// Determine version based on file type
				fullVersion := r.pkg.Metadata["fullVersion"]
				if strings.Contains(srcFileName, ".orig.tar.") {
					// Extract upstream version from full version (e.g., "2.4.52-1" -> "2.4.52")
					srcMetadata["version"] = artifact.ExtractUpstreamVersion(fullVersion)
				} else {
					// For .debian.tar.*, use full version
					srcMetadata["version"] = fullVersion
				}

				// Upload the source file
				// Use just the filename (not the full path) for the upload
				err = r.destAdapter.UploadFile(r.destRegistry, srcFile, &types.File{Name: srcFileName, Uri: srcFilePath}, srcHeader,
					srcFileName, srcFileName, r.artifactType, srcMetadata)
				srcFile.Close()

				srcStat := types.FileStat{
					Name:     srcFileName,
					Registry: r.srcRegistry,
					Uri:      srcFilePath,
					Size:     0, // Size not available here
					Status:   types.StatusSuccess,
				}
				if err != nil {
					r.logger.Error().Err(err).Msgf("Failed to upload source file %s", srcFileName)
					srcStat.Status = types.StatusFail
					srcStat.Error = err.Error()
					pterm.Error.Println(srcTitle)
				} else {
					pterm.Success.Println(srcTitle)
				}
				r.stats.FileStats = append(r.stats.FileStats, srcStat)
			}
		}
	}

	return nil
}

func (r *Package) migrateComposer(ctx context.Context) error {
	if r.config.DryRun {
		log.Info().Ctx(ctx).Msgf("Dry-run: would migrate Composer package %s", r.pkg.URL)
		return nil
	}
	file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.URL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download Composer package %s", r.pkg.URL)
		pterm.Error.Println(fmt.Sprintf("Failed to download Composer package %s", r.pkg.URL))
		return err
	}
	defer file.Close()

	title := fmt.Sprintf("%s (%s)", r.pkg.Name, common.GetSize(int64(r.pkg.Size)))
	pterm.Info.Println(fmt.Sprintf("Copying file %s from %s to %s", r.pkg.Name, r.srcRegistry, r.destRegistry))
	err = r.destAdapter.UploadFile(r.destRegistry, file, &types.File{Uri: r.pkg.URL}, header, r.pkg.Name, r.pkg.Version,
		r.artifactType, nil)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     int64(r.pkg.Size),
		Status:   types.StatusSuccess,
	}
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to upload file")
		stat.Status = types.StatusFail
		stat.Error = err.Error()
		pterm.Error.Println(title)
	} else {
		pterm.Success.Println(title)
	}
	r.stats.FileStats = append(r.stats.FileStats, stat)
	return nil
}

func (r *Package) migrateSwift(ctx context.Context) error {
	if r.config.DryRun {
		log.Info().Ctx(ctx).Msgf("Dry-run: would migrate Swift package %s", r.pkg.URL)
		return nil
	}
	file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.pkg.URL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to download Swift package %s", r.pkg.URL)
		pterm.Error.Println(fmt.Sprintf("Failed to download Swift package %s", r.pkg.URL))
		return err
	}
	defer file.Close()

	title := fmt.Sprintf("%s (%s)", r.pkg.Name, common.GetSize(int64(r.pkg.Size)))
	pterm.Info.Println(fmt.Sprintf("Copying file %s from %s to %s", r.pkg.Name, r.srcRegistry, r.destRegistry))
	err = r.destAdapter.UploadFile(r.destRegistry, file, &types.File{Uri: r.pkg.URL}, header, r.pkg.Name, r.pkg.Version,
		r.artifactType, nil)
	stat := types.FileStat{
		Name:     r.pkg.Name,
		Registry: r.srcRegistry,
		Uri:      r.pkg.URL,
		Size:     int64(r.pkg.Size),
		Status:   types.StatusSuccess,
	}
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to upload file")
		stat.Status = types.StatusFail
		stat.Error = err.Error()
		pterm.Error.Println(title)
	} else {
		pterm.Success.Println(title)
	}
	r.stats.FileStats = append(r.stats.FileStats, stat)
	return nil
}

// migrateConan migrates every file of a single Conan reference
// (name/version[@user/channel]). The reference subtree (r.node) is parsed into
// recipe- and package-layer files, which are uploaded in an order that places
// each conanmanifest.txt last within its revision group (the finalization
// marker the server expects last). The source SHA1 from the JFrog listing is
// forwarded so the destination can verify each upload.
func (r *Package) migrateConan(ctx context.Context) error {
	if r.config.DryRun {
		r.logger.Info().Msgf("Dry-run: skipping Conan migration for reference %s", r.pkg.Name)
		return nil
	}

	files, err := tree.GetAllFiles(r.node)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to list files for Conan reference %s", r.pkg.Name)
		return fmt.Errorf("get files for conan reference %s: %w", r.pkg.Name, err)
	}

	entries := util.ParseConanEntries(files)
	if len(entries) == 0 {
		r.logger.Warn().Msgf("No Conan files found for reference %s", r.pkg.Name)
		return nil
	}

	for _, entry := range entries {
		file, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, entry.Uri)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("Failed to download Conan file %s", entry.Uri)
			pterm.Error.Println(fmt.Sprintf("Failed to download Conan file %s", entry.Uri))
			r.stats.FileStats = append(r.stats.FileStats, types.FileStat{
				Name:     entry.FileName,
				Registry: r.srcRegistry,
				Uri:      entry.Uri,
				Size:     int64(entry.Size),
				Status:   types.StatusFail,
				Error:    err.Error(),
			})
			continue
		}

		metadata := map[string]interface{}{
			"layer":    string(entry.Layer),
			"name":     entry.Reference.Name,
			"version":  entry.Reference.Version,
			"user":     entry.Reference.User,
			"channel":  entry.Reference.Channel,
			"rrev":     entry.RRev,
			"pkgid":    entry.PkgID,
			"prev":     entry.PRev,
			"filename": entry.FileName,
			"sha1":     entry.SHA1,
		}

		title := fmt.Sprintf("%s (%s)", entry.FileName, common.GetSize(int64(entry.Size)))
		pterm.Info.Println(fmt.Sprintf("Copying Conan file %s from %s to %s", entry.FileName, r.srcRegistry, r.destRegistry))
		err = r.destAdapter.UploadFile(r.destRegistry, file, &types.File{Name: entry.FileName, Uri: entry.Uri}, header,
			entry.Reference.Name, entry.Reference.Version, types.CONAN, metadata)

		stat := types.FileStat{
			Name:     entry.FileName,
			Registry: r.srcRegistry,
			Uri:      entry.Uri,
			Size:     int64(entry.Size),
			Status:   types.StatusSuccess,
		}
		if err != nil {
			if errors.Is(err, types.ErrArtifactAlreadyExists) {
				stat.Status = types.StatusSkip
				pterm.Info.Println(fmt.Sprintf("%s already exists, skipping", title))
			} else {
				r.logger.Error().Err(err).Msgf("Failed to upload Conan file %s", entry.FileName)
				stat.Status = types.StatusFail
				stat.Error = err.Error()
				pterm.Error.Println(title)
			}
		} else {
			pterm.Success.Println(title)
		}
		r.stats.FileStats = append(r.stats.FileStats, stat)
	}

	return nil
}

const labelMaxBytes = 1024

func readChartMeta(path string) (*chart.Metadata, error) {
	ch, err := loader.Load(path) // understands .tgz & directories
	if err != nil {
		log.Error().Msgf("Failed to load chart metadata from %s", path)
	}
	if ch == nil || ch.Metadata == nil {
		return &chart.Metadata{}, err
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
	if meta.Maintainers != nil {
		if b, _ := json.Marshal(meta.Maintainers); len(b) > 0 {
			lbl["chart.maintainers"] = truncate(string(b))
		}
	}

	if meta.Dependencies != nil {
		if b, _ := json.Marshal(meta.Dependencies); len(b) > 0 {
			lbl["chart.dependencies"] = truncate(string(b))
		}
	}

	if meta.Annotations != nil {
		if b, _ := json.Marshal(meta.Annotations); len(b) > 0 {
			lbl["chart.annotations"] = truncate(string(b))
		}
	}
	return lbl
}

// pushChart uploads chart.tar.gz --> oci://<dstRef>
func (r *Package) pushChart(ctx context.Context, chartPath string, dstRef string) error {
	meta, err := readChartMeta(chartPath)
	if err != nil {
		log.Error().Msgf("Failed to read chart metadata from %s", chartPath)
		return errors.New("failed to read chart metadata from chartPath")
	}
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
	img = mutate.MediaType(img, types2.OCIManifestSchema1)

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
	keyChain, err := lib.CreateCraneKeychain(r.srcAdapter, r.destAdapter, r.sourcePackageHostname)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("Failed to create keyChain: %v", err)
		pterm.Error.Println(fmt.Sprintf("Failed to create keyChain: %v", err))
		return err
	}

	craneOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithUserAgent("harness-cli"),
		remote.WithAuthFromKeychain(keyChain),
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

// addPackageToDryRunDirectory adds package to the directory structure
func (r *Package) addPackageToDryRunDirectory() {
	r.dryRunStats.EnsurePackage(r.srcRegistry, r.pkg.Name)
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
