package migratable

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/tree"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/util"

	"github.com/google/uuid"
	"github.com/pterm/pterm"
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
	dryRunStats           *types.DryRunStats

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
	dryRunStats *types.DryRunStats,
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
		dryRunStats:           dryRunStats,
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

	// Skip destination registry check in dry-run mode
	if r.config.DryRun {
		logger.Info().Msg("Dry-run mode: skipping destination registry check")
		r.registry = types.RegistryInfo{
			Path: r.destRegistry,
		}
		logger.Info().
			Dur("duration", time.Since(startTime)).
			Msg("Completed registry pre-migration step (dry-run)")
		return nil
	}

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
		logger.Error().Msgf("Either include or Exclude Pattern is supported at a time for %s", r.artifactType)
		return fmt.Errorf("failed in validating config file for %s ", r.artifactType)
	}

	startTime := time.Now()

	fetchProgress := startStage(fmt.Sprintf("Fetching file metadata from source registry %s", r.srcRegistry))
	files, err2 := r.srcAdapter.GetFiles(r.srcRegistry)
	if err2 != nil {
		fetchProgress.fail(fmt.Sprintf("Failed to fetch file metadata from source registry %s", r.srcRegistry))
		logger.Error().Msgf("Failed to get files from registry %s", r.srcRegistry)
		util.AddRegistryErrorToStat(r.stats, r.srcRegistry, err2)
		return fmt.Errorf("get files from registry %s failed: %w", r.srcRegistry, err2)
	}
	pulledMsg := fmt.Sprintf("Pulled %d file(s) from registry %s", len(files), r.srcRegistry)
	logger.Info().Msg(pulledMsg)
	fetchProgress.success(pulledMsg)
	// originalFiles is the pristine GetFiles listing, before date/pattern
	// narrowing. Used to build the recovery tree for atomic-version types below.
	originalFiles := files

	// In dry-run mode, collect all files and initialize directory entry for this registry
	if r.config.DryRun && r.dryRunStats != nil {
		// Add all files to the file list
		entries := make([]types.DryRunFileEntry, 0, len(files))
		for _, file := range files {
			entries = append(entries, types.DryRunFileEntry{
				Registry:     r.srcRegistry,
				Name:         file.Name,
				Uri:          file.Uri,
				Size:         file.Size,
				LastModified: file.LastModified,
			})
		}
		r.dryRunStats.AddFiles(entries...)
		logger.Info().Msgf("Dry-run: collected %d files from registry %s", len(files), r.srcRegistry)

		// Initialize directory entry for this registry
		r.dryRunStats.EnsureRegistry(r.srcRegistry)
	}

	currArtifactType := r.artifactType

	// dateFilteredFiles holds the date-filtered file list (may be empty/nil when nothing matches).
	// For file-level artifact types it is also applied to the tree.
	// For metadata-driven types (RPM, DEBIAN) it is used after GetPackages.
	var dateFilteredFiles []types.File
	dateFilterActive := false

	if util.IsTimeBasedFilterPresent(r.mapping) {
		dateFilterActive = true

		df := r.mapping.DateFilter
		if err := util.ValidateDateFilter(df); err != nil {
			logger.Error().Err(err).Msg("Date filter validation failed")
			return err
		}

		searchedFiles, err2 := r.srcAdapter.SearchFiles(r.srcRegistry)
		if err2 != nil {
			logger.Error().Msgf("Failed to search files from registry %s", r.srcRegistry)
			util.AddRegistryErrorToStat(r.stats, r.srcRegistry, err2)
			return fmt.Errorf("search files from registry %s failed: %w", r.srcRegistry, err2)
		}
		logger.Info().Msgf("Applying time based filter (match: %s)", df.Match)
		filteredURIs := util.CreateMapOfFilteredFile(searchedFiles, r.mapping)
		logger.Info().Msgf("Time-based filter include %d file(s): out of  %d", len(filteredURIs), len(searchedFiles))
		// Always keep repository index/metadata files (e.g. PyPI's .pypi/*.html)
		// regardless of the date filter: enumeration reads them to list packages
		// and versions, and they are typically too old to survive a
		// createdAfter/downloadedAfter cutoff. Dropping them would break
		// enumeration for the whole registry.
		indexCount := 0
		for _, f := range files {
			if util.IsPackageIndexFile(r.artifactType, f.Uri) {
				if _, ok := filteredURIs[f.Uri]; !ok {
					filteredURIs[f.Uri] = struct{}{}
					indexCount++
				}
			}
		}
		if indexCount > 0 {
			logger.Info().Msgf("Preserving %d index/metadata file(s) exempt from date filter", indexCount)
		}

		dateFilteredFiles = util.FilterFilesByDate(files, filteredURIs)
		logger.Info().Msgf("Count of filtered files by date filter: %d -> %d", len(files), len(dateFilteredFiles))

		// Narrow the tree for all types EXCEPT metadata-driven types (RPM, DEBIAN)
		// which need full file trees to read metadata files (repomd.xml, Packages.gz, etc.).
		// Date filtering for metadata-driven types is applied after GetPackages.
		if !util.IsMetadataDrivenArtifact(currArtifactType) {
			files = dateFilteredFiles
		}
	}

	// Filter files based on include/exclude patterns
	if util.IsFileLevelFilterableArtifact(currArtifactType) {
		if len(r.mapping.IncludePatterns) > 0 || len(r.mapping.ExcludePatterns) > 0 {
			originalCount := len(files)
			filteredFiles := util.FilterFilesByPatterns(files, r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
			files = filteredFiles
			logger.Info().Msgf("Filtered files: %d -> %d (includePatterns: %v, excludePatterns: %v)",
				originalCount, len(files), r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
		}
	}

	skippedByFilter := len(originalFiles) - len(files)
	skipMsg := fmt.Sprintf("Registry %s: %d file(s) pulled, %d under skip condition (date/pattern filters)",
		r.srcRegistry, len(originalFiles), skippedByFilter)
	logger.Info().Msg(skipMsg)
	pterm.Info.Println(skipMsg)

	// For atomic-version types (PyPI), a version is migrated in full if ANY of
	// its files is in window. buildVersionJobs recovers a version's pruned
	// distribution files from this unfiltered tree. It is pattern-filtered (never
	// resurrect an explicitly excluded file) but NOT date-filtered.
	// TERRAFORM also gets the unfiltered tree: its provider versions are atomic
	// multi-file versions too (all platform zips per version), but discovery
	// happens inside Version.Migrate, which swaps to this tree — see version.go.
	var unfilteredRoot *types.TreeNode
	if dateFilterActive && (util.IsAtomicVersionArtifact(currArtifactType) || currArtifactType == types.TERRAFORM) {
		recoveryFiles := originalFiles
		if util.IsFileLevelFilterableArtifact(currArtifactType) &&
			(len(r.mapping.IncludePatterns) > 0 || len(r.mapping.ExcludePatterns) > 0) {
			recoveryFiles = util.FilterFilesByPatterns(originalFiles, r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
		}
		unfilteredRoot = tree.TransformToTree(recoveryFiles)
	}

	root := tree.TransformToTree(files)

	pkgs, err := r.srcAdapter.GetPackages(r.srcRegistry, r.artifactType, root)
	if err != nil {
		logger.Error().Msg("Failed to get packages")
		util.AddRegistryErrorToStat(r.stats, r.srcRegistry, err)
		return fmt.Errorf("get packages failed: %w", err)
	}

	// Guard: if the source registry had files but zero packages were resolved,
	// the artifact type is likely misconfigured or unsupported for this source.
	// Treat this as an error so the caller exits non-zero instead of silently
	// reporting "Migration completed successfully" with 0 artifacts transferred.
	//
	// The guard fires whenever the enumeration tree still had input: filters
	// (date/pattern) can legitimately starve the tree to ZERO files — a valid
	// empty result — but if files SURVIVED the filters and GetPackages still
	// resolved nothing, the surviving content does not parse as this artifact
	// type, which is a type mismatch no filter setting can explain away.
	// (Package-level pattern filters and packageFilters are applied AFTER this
	// point, so they can never be the cause of a zero here. For
	// metadata-driven types the tree is intentionally not date-narrowed, so
	// `files` still reflects the full listing for them.)
	if len(pkgs) == 0 && len(files) > 0 {
		err := fmt.Errorf(
			"registry %s: pulled %d file(s) from source (%d after filters) but resolved 0 packages for artifactType %s — "+
				"verify the registry contains %s artifacts and the artifactType is correct",
			r.srcRegistry, len(originalFiles), len(files), r.artifactType, r.artifactType,
		)
		util.AddRegistryErrorToStat(r.stats, r.srcRegistry, err)
		return err
	}

	// For metadata-driven artifact types (RPM, DEBIAN), GetPackages reads a
	// repository metadata file that lists every package regardless of the
	// filtered tree.  Re-apply the date filter at the package level by
	// matching each package's bare filename against the date-filtered file list.
	if dateFilterActive && util.IsMetadataDrivenArtifact(currArtifactType) {
		originalPkgCount := len(pkgs)
		pkgs = util.FilterPackagesByFileName(pkgs, dateFilteredFiles)
		logger.Info().Msgf("Date filter (post-GetPackages): %d -> %d packages for %s",
			originalPkgCount, len(pkgs), currArtifactType)
	}

	// applying package level filter
	composerPkgsBeforeFilters := 0
	if r.artifactType == types.COMPOSER {
		composerPkgsBeforeFilters = len(pkgs)
	}
	if util.IsPackageLevelFilterableArtifact(currArtifactType) {
		if len(r.mapping.IncludePatterns) > 0 || len(r.mapping.ExcludePatterns) > 0 {
			originalCount := len(pkgs)
			filteredPackages := util.FilterFilesByPatternsPackageName(pkgs, r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
			pkgs = filteredPackages
			logger.Info().Msgf("Filtered packages: %d -> %d (includePatterns: %v, excludePatterns: %v)",
				originalCount, len(pkgs), r.mapping.IncludePatterns, r.mapping.ExcludePatterns)
		}
	}

	// Apply the opt-in package selector allow-list (packageFilters). When set,
	// only the named packages are migrated; runs before the destination index is
	// built so the index only covers selected work. No-op when unset.
	if len(r.mapping.PackageFilters) > 0 {
		originalCount := len(pkgs)
		pkgs = util.FilterPackagesBySelectors(pkgs, r.mapping.PackageFilters)
		logger.Info().Msgf("Package selector filter: %d -> %d packages", originalCount, len(pkgs))
	}

	if r.artifactType == types.COMPOSER &&
		composerPkgsBeforeFilters > 0 &&
		len(pkgs) == 0 &&
		(len(r.mapping.IncludePatterns) > 0 || len(r.mapping.ExcludePatterns) > 0 || len(r.mapping.PackageFilters) > 0) {
		warnMsg := fmt.Sprintf(
			"Registry %s: Composer package filters reduced %d package(s) to 0; nothing will be migrated — "+
				"use vendor/package names (e.g. harness/migtest, acme/*), not zip basenames, in includePatterns, excludePatterns, and packageFilters",
			r.srcRegistry, composerPkgsBeforeFilters,
		)
		logger.Warn().Msg(warnMsg)
		pterm.Warning.Println(warnMsg)
	}

	// Build destination index once per registry when overwrite=false
	var existingIndex *types.ExistingIndex
	if !r.config.Overwrite && !r.config.DryRun && indexApplicable(r.artifactType) {
		name := registryLeafName(r.destRegistry, r.registry.Path)
		indexProgress := startStage(fmt.Sprintf("Indexing artifacts already in destination registry %s", name))
		idx, err := r.destAdapter.BuildExistingIndex(ctx, name, r.config.Concurrency)
		if err != nil {
			indexProgress.warn(fmt.Sprintf(
				"Could not index destination registry %s; falling back to per-version lookups", name))
			logger.Warn().Err(err).Msg("Failed to build destination index; falling back to per-version lookups")
		} else {
			existingIndex = idx
			indexedPkgs, indexedVersions, indexedFiles := idx.Stats()
			indexedMsg := fmt.Sprintf(
				"Indexed destination registry %s: %d package(s), %d version(s), %d existing file(s)",
				name, indexedPkgs, indexedVersions, indexedFiles)
			indexProgress.success(indexedMsg)
			logger.Info().Msg(indexedMsg)
		}
	}

	var jobs []engine.Job
	for _, pkg := range pkgs {
		treeNode, err2 := tree.GetNodeForPath(root, pkg.Path)
		if err2 != nil {
			logger.Error().Msgf("Failed to get node for path %s", pkg.Path)
			return fmt.Errorf("get node for path %s failed: %w", pkg.Path, err2)
		}
		job := NewPackageJob(r.srcAdapter, r.destAdapter, r.srcRegistry, r.sourcePackageHostname, r.destRegistry, r.artifactType, pkg, treeNode, unfilteredRoot,
			r.stats, r.mapping, r.config, r.registry, r.dryRunStats, existingIndex)
		jobs = append(jobs, job)
	}

	eng := engine.NewEngine(r.config.Concurrency, jobs)
	err = eng.Execute(ctx)
	if err != nil {
		// Propagate so the failure reaches MigrationService.Run's exit-code
		// contract; the per-coordinate stats already carry the detail.
		logger.Error().Err(err).Msg("Engine execution saw following errors")
		return fmt.Errorf("registry %s: package migration errors: %w", r.srcRegistry, err)
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

// indexApplicable reports whether Version.Pre consults the file index for this type.
// Must exactly equal the set Version.Pre checks today (version.go:109 exclusions).
func indexApplicable(t types.ArtifactType) bool {
	switch t {
	case types.GENERIC, types.RAW, types.PYTHON, types.NUGET, types.DART, types.PUPPET, types.NPM, types.MAVEN:
		return true
	default:
		return false
	}
}

// registryLeafName extracts the registry leaf name for v3 API matching.
func registryLeafName(destRegistry, regPath string) string {
	if regPath != "" {
		return path.Base(regPath)
	}
	return destRegistry
}
