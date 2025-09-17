package migratable

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harness/harness-cli/module/ar/migrate/adapter"
	"github.com/harness/harness-cli/module/ar/migrate/engine"
	"github.com/harness/harness-cli/module/ar/migrate/types"
	"github.com/harness/harness-cli/module/ar/migrate/types/npm"
	"github.com/harness/harness-cli/module/ar/migrate/util"
	"github.com/harness/harness-cli/util/common"

	"github.com/google/uuid"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/blake2b"
)

type File struct {
	srcRegistry   string
	destRegistry  string
	srcAdapter    adapter.Adapter
	destAdapter   adapter.Adapter
	artifactType  types.ArtifactType
	logger        zerolog.Logger
	pkg           types.Package
	version       types.Version
	file          *types.File
	node          *types.TreeNode
	stats         *types.TransferStats
	skipMigration bool
	mapping       *types.RegistryMapping
	config        *types.Config
	registry      types.RegistryInfo
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
	stats *types.TransferStats,
	mapping *types.RegistryMapping,
	config *types.Config,
	registry types.RegistryInfo,
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
		stats:        stats,
		mapping:      mapping,
		config:       config,
		registry:     registry,
	}
}

func (r *File) Info() string {
	info := r.srcRegistry + ":" + r.destRegistry + ":" + r.pkg.Name + ":" + r.version.Name
	if r.file != nil {
		info += ":" + r.file.Name
	}

	return info
}

func (r *File) Pre(ctx context.Context) error {
	// Extract trace ID from context if available
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "pre").
		Str("trace_id", traceID).
		Logger()
	logger.Info().Msg("Starting version pre-migration step")
	startTime := time.Now()

	if !r.config.Overwrite && (r.artifactType != types.MAVEN && r.pkg.Name != "" && r.version.Name != "" && r.file.Name != "") {
		exists, err := r.destAdapter.FileExists(ctx,
			r.registry.Path,
			r.pkg.Name, r.version.Name, r.file.Name, r.artifactType)
		if err != nil {
			log.Error().Err(err).Msg("Failed to check if version exists")
			return nil
		}
		if exists {
			util.GetSkipPrinter().Println(fmt.Sprintf("Registry [%s], Package [%s/%s], File [%s] already exists",
				r.destRegistry,
				r.pkg.Name, r.version.Name, r.file.Name))
			r.skipMigration = true
			stat := types.FileStat{
				Name:     r.file.Name,
				Registry: r.srcRegistry,
				Uri:      r.file.Uri,
				Size:     int64(r.file.Size),
				Status:   types.StatusSkip,
			}
			r.stats.FileStats = append(r.stats.FileStats, stat)
			return nil
		}
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed file pre-migration step")
	return nil
}

func (r *File) Migrate(ctx context.Context) error {
	traceID, _ := ctx.Value("trace_id").(string)
	logger := r.logger.With().
		Str("step", "migrate").
		Str("trace_id", traceID).
		Logger()

	logger.Info().Msg("Starting file migration step")
	startTime := time.Now()

	if r.skipMigration {
		return nil
	}

	if r.artifactType == types.DOCKER || r.artifactType == types.HELM {
		log.Error().Ctx(ctx).Msgf("OCI migrate file is not supported")
		return fmt.Errorf("OCI migrate file is not supported")
	}

	if r.artifactType == types.GENERIC || r.artifactType == types.MAVEN || r.artifactType == types.NUGET {
		downloadFile, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.file.Uri)
		defer downloadFile.Close()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to download file")
			return fmt.Errorf("download file failed: %w", err)
		}

		//readCloser := progress.ReadCloser(int64(r.file.Size), downloadFile, r.file.Name)
		title := fmt.Sprintf("%s (%s)", r.file.Name, common.GetSize(int64(r.file.Size)))
		pterm.Info.Println(fmt.Sprintf("Copying file %s from %s to %s", r.file.Name, r.srcRegistry, r.destRegistry))
		err = r.destAdapter.UploadFile(r.destRegistry, downloadFile, r.file, header, r.pkg.Name, r.version.Name,
			r.artifactType, nil)
		stat := types.FileStat{
			Name:     r.file.Name,
			Registry: r.srcRegistry,
			Uri:      r.file.Uri,
			Size:     int64(r.file.Size),
			Status:   types.StatusSuccess,
		}
		if err != nil {
			logger.Error().Err(err).Msg("Failed to upload file")
			stat.Status = types.StatusFail
			stat.Error = err.Error()
			pterm.Error.Println(title)
		} else {
			pterm.Success.Println(title)
		}
		r.stats.FileStats = append(r.stats.FileStats, stat)
	}

	if r.artifactType == types.PYTHON {
		downloadFile, header, err := r.srcAdapter.DownloadFile(r.srcRegistry, r.file.Uri)
		tempFile, err := os.CreateTemp("", fmt.Sprintf("python-pkg-%s-*", r.file.Name))
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create temporary file")
			return fmt.Errorf("failed to create temporary file: %w", err)
		}
		defer os.Remove(tempFile.Name()) // Clean up the temp file when done

		// Copy the downloaded content to the temp file
		_, err = io.Copy(tempFile, downloadFile)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to write to temporary file")
			return fmt.Errorf("failed to write to temporary file: %w", err)
		}
		err = tempFile.Close()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to close temporary file")
			return fmt.Errorf("failed to close temporary file: %w", err)
		}

		// Check file extension to determine package type
		fileName := r.file.Name
		title := fmt.Sprintf("%s (%s)", fileName, common.GetSize(int64(r.file.Size)))

		var metadata string
		// Extract metadata based on file extension
		if strings.HasSuffix(fileName, ".tar.gz") ||
			strings.HasSuffix(fileName, ".tgz") ||
			strings.HasSuffix(fileName, ".zip") {
			metadata, err = r.extractTarGzMetadataFile(tempFile.Name())
			if err != nil {
				logger.Error().Err(err).Msg("Failed to extract metadata from tar.gz file")
				return fmt.Errorf("failed to extract metadata from tar.gz file: %w", err)
			}
		} else if strings.HasSuffix(fileName, ".whl") {
			metadata, err = r.extractWheelMetadataFile(tempFile.Name())
			if err != nil {
				logger.Error().Err(err).Msg("Failed to extract metadata from wheel file")
				return fmt.Errorf("failed to extract metadata from wheel file: %w", err)
			}
		} else {
			logger.Warn().Msg("Unsupported Python package format, uploading without metadata extraction")
		}

		metadataMap, err := generatePythonMetadataMap(metadata, tempFile.Name())
		if err != nil {
			logger.Error().Err(err).Msg("Failed to generate metadata map")
			return fmt.Errorf("failed to generate metadata map: %w", err)
		}

		tempFileReader, err := os.Open(tempFile.Name())
		if err != nil {
			logger.Error().Err(err).Msg("Failed to open temporary file")
			return fmt.Errorf("failed to open temporary file: %w", err)
		}
		defer tempFileReader.Close()

		err = r.destAdapter.UploadFile(r.destRegistry, tempFileReader, r.file, header, r.pkg.Name, r.version.Name,
			r.artifactType, metadataMap)

		stat := types.FileStat{
			Name:     r.file.Name,
			Registry: r.srcRegistry,
			Uri:      r.file.Uri,
			Size:     int64(r.file.Size),
			Status:   types.StatusSuccess,
		}
		if err != nil {
			logger.Error().Err(err).Msg("Failed to upload file")
			stat.Status = types.StatusFail
			stat.Error = err.Error()
			pterm.Error.Println(title)
		} else {
			pterm.Success.Println(title)
		}
		r.stats.FileStats = append(r.stats.FileStats, stat)
	} else if r.artifactType == types.NPM {
		if !strings.Contains(r.file.Uri, "/-/") {
			logger.Error().Msg("File Download url is not correct for NPM")
			return fmt.Errorf("file Download url is not correct for NPM")
		}

		// Truncate everything after the last "/-/" segment
		idx := strings.LastIndex(r.file.Uri, "/-/")
		if idx == -1 {
			return fmt.Errorf("file Download url is not correct for NPM")
		}
		metadataUrl := r.file.Uri[:idx]
		pkgMetadata, _, err := r.srcAdapter.DownloadFile(r.srcRegistry, metadataUrl)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to fetch metadata")
			return fmt.Errorf("failed to fetch metadata: %w", err)
		}
		var metadata npm.PackageMetadata
		if err := json.NewDecoder(pkgMetadata).Decode(&metadata); err != nil {
			logger.Error().Err(err).Msg("Failed to parse NPM metadata")
			return fmt.Errorf("fFailed to parse NPM metadata: %w", err)
		}
		var tarballURL string
		for _, p := range metadata.Versions {
			if p.Version == r.version.Name {
				metadata.Versions = map[string]*npm.PackageMetadataVersion{p.Version: p}
				tarballURL = p.Dist.Tarball
				break
			}
		}

		// Prepare package upload object
		upload := &npm.PackageUpload{
			PackageMetadata: metadata, // already filtered to 1 version
			Attachments:     make(map[string]*npm.PackageAttachment),
		}

		file, _, err := r.srcAdapter.DownloadFile(r.srcRegistry, tarballURL)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to download NPM tarball")
			return fmt.Errorf("failed to download NPM tarball: %w", err)
		}
		defer file.Close()

		uploadReader, err2 := r.ParseNPMetadata(err, file, logger, tarballURL, upload)
		if err2 != nil {
			return err2
		}

		err = r.destAdapter.UploadFile(r.destRegistry, uploadReader, r.file, nil, r.pkg.Name, r.version.Name,
			r.artifactType, nil)

		if err != nil {
			return err
		}

		// Add all dist-tags from metadata
		for tagName, tagVersion := range metadata.DistTags {
			if tagVersion == r.version.Name {
				distTagsUri := r.file.Uri
				if idx := strings.LastIndex(distTagsUri, "/-/"); idx != -1 {
					distTagsUri = "/-/package/" + r.pkg.Name + "/dist-tags"
				}

				distTagsUri = distTagsUri + "/" + tagName
				err = r.destAdapter.AddNPMTag(r.destRegistry, r.pkg.Name, tagVersion, distTagsUri)
				if err != nil {
					logger.Error().Err(err).Msgf("Failed to add NPM tag %s: %s", tagName, tagVersion)
					// Continue with other tags even if one fails
				} else {
					logger.Info().Msgf("Successfully added NPM tag %s: %s", tagName, tagVersion)
				}
				break
			}
		}

		stat := types.FileStat{
			Name:     r.file.Name,
			Registry: r.srcRegistry,
			Uri:      r.file.Uri,
			Size:     int64(r.file.Size),
			Status:   types.StatusSuccess,
		}
		title := fmt.Sprintf("%s (%s)", r.file.Name, common.GetSize(int64(r.file.Size)))
		if err != nil {
			logger.Error().Err(err).Msg("Failed to upload file")
			stat.Status = types.StatusFail
			stat.Error = err.Error()
			pterm.Error.Println(title)
		} else {
			pterm.Success.Println(title)
		}
		r.stats.FileStats = append(r.stats.FileStats, stat)
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Completed version migration step")
	return nil
}

func (r *File) ParseNPMetadata(
	err error,
	file io.ReadCloser,
	logger zerolog.Logger,
	tarballURL string,
	upload *npm.PackageUpload,
) (io.ReadCloser, error) {
	var buf bytes.Buffer
	size, err := io.Copy(&buf, file)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read tarball stream")
		return nil, fmt.Errorf("failed to read tarball stream: %w", err)
	}

	// Step 4: Base64 encode
	b64Data := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Step 5: Create PackageAttachment
	tarballName := filepath.Base(tarballURL)
	upload.Attachments[tarballName] = &npm.PackageAttachment{
		ContentType: "application/octet-stream",
		Data:        b64Data,
		Length:      int(size),
	}
	uploadReader := io.NopCloser(StreamUploadAsJSON(upload))
	return uploadReader, nil
}

func StreamUploadAsJSON(upload *npm.PackageUpload) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		encoder := json.NewEncoder(pw)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(upload); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to encode upload JSON: %w", err))
		}
	}()

	return pr
}

func generatePythonMetadataMap(metadata string, path string) (map[string]interface{}, error) {
	mapData := make(map[string]interface{})
	msg, err := mail.ReadMessage(bytes.NewReader([]byte(metadata)))
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}
	for key, h := range msg.Header {
		lowerKey := strings.ToLower(key)
		lowerKey = strings.ReplaceAll(lowerKey, "-", "_")
		if lowerKey == "platform" || lowerKey == "supported_platform" || lowerKey == "classifier" || lowerKey == "provides_extra" {
			lowerKey += "s"
		}

		if lowerKey == "project_url" || lowerKey == "project_urls" {
			continue
		}

		if len(h) == 1 {
			mapData[lowerKey] = h[0]
		}
		mapData[lowerKey] = h
	}
	all, err := io.ReadAll(msg.Body)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read metadata")
	}
	mapData["description"] = string(all)
	mapData["description_content_type"] = "text/markdown"

	// Calculate file digests
	file, err := os.Open(path)
	if err != nil {
		return mapData, fmt.Errorf("failed to open file for digest calculation: %w", err)
	}
	defer file.Close()

	// Create hash instances
	md5Hash := md5.New()
	sha256Hash := sha256.New()
	blake2bHash, err := blake2b.New256(nil)
	if err != nil {
		return mapData, fmt.Errorf("failed to create blake2b hash: %w", err)
	}

	// Create a multi-writer to write to all hash functions at once
	multiWriter := io.MultiWriter(md5Hash, sha256Hash, blake2bHash)

	// Copy the file content to the hasher
	if _, err := io.Copy(multiWriter, file); err != nil {
		return mapData, fmt.Errorf("failed to calculate digests: %w", err)
	}

	// Add the digests to the metadata map
	mapData["md5_digest"] = hex.EncodeToString(md5Hash.Sum(nil))
	mapData["sha256_digest"] = hex.EncodeToString(sha256Hash.Sum(nil))
	mapData["blake2_256_digest"] = hex.EncodeToString(blake2bHash.Sum(nil))

	return mapData, nil
}

func (r *File) Post(ctx context.Context) error {
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

// extractTarGzMetadataFile extracts metadata from a tar.gz Python package
func (r *File) extractTarGzMetadataFile(path string) (string, error) {
	file, err2 := os.Open(path)
	defer file.Close()

	if err2 != nil {
		return "", fmt.Errorf("failed to read file: %w", err2)
	}
	var buf bytes.Buffer
	tee := io.TeeReader(file, &buf)
	// Create a new gzip reader
	gzipReader, err := gzip.NewReader(tee)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create a new tar reader
	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar header: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		fileName := filepath.Base(header.Name)

		// Check if this file is a metadata file
		if fileName == "PKG-INFO" || fileName == "METADATA" {
			all, err := io.ReadAll(tarReader)
			if err != nil {
				return "", fmt.Errorf("failed to read metadata file: %w", err)
			}
			return string(all), nil
		}
	}
	return "", fmt.Errorf("metadata file not found")
}

// extractWheelMetadataFile extracts metadata from a wheel Python package
func (r *File) extractWheelMetadataFile(path string) (string, error) {
	file, err2 := os.Open(path)
	if err2 != nil {
		return "", fmt.Errorf("failed to read file: %w", err2)
	}
	defer file.Close()
	var buf bytes.Buffer
	tee := io.TeeReader(file, &buf)

	// Create a new zip reader
	data, err := io.ReadAll(tee)
	if err != nil {
		return "", fmt.Errorf("failed to read wheel file: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("failed to create zip reader: %w", err)
	}

	// Look for metadata and README files in the zip archive
	for _, file := range zipReader.File {
		fileName := filepath.Base(file.Name)

		// Check if this file is a metadata file
		if fileName == "METADATA" || fileName == "PKG-INFO" {
			metadataFile, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open metadata file: %w", err)
			}
			defer metadataFile.Close()
			all, err := io.ReadAll(metadataFile)
			if err != nil {
				return "", fmt.Errorf("failed to read metadata file: %w", err)
			}
			return string(all), nil
		}
		continue
	}

	return "", fmt.Errorf("no metadata file found in the package")
}
