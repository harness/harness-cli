package command

import (
	"context"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/metadata"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// applyPostPushMetadata attaches key-value metadata to an artifact version after a successful push.
// metadataStr uses the same "key:value,key2:value2" format as `hc artifact metadata set --metadata`.
// It is a no-op when metadataStr is empty or pkg is unknown.
// Failures are logged as warnings and do not fail the push.
func applyPostPushMetadata(f *cmdutils.Factory, metadataStr, registry, pkg, version string) {
	if metadataStr == "" {
		return
	}
	if pkg == "" {
		logrus.Warnf("Skipping metadata publish: package name is unknown for this package type; pass --metadata only when the package name is resolvable")
		return
	}
	items, err := metadata.ParseMetadataString(metadataStr)
	if err != nil {
		logrus.Warnf("Failed to parse --metadata for %s/%s@%s: %v", registry, pkg, version, err)
		return
	}
	params := &ar_v2.UpdateMetadataParams{
		AccountIdentifier: config.Global.AccountID,
	}
	body := ar_v2.UpdateMetadataJSONRequestBody{
		RegistryIdentifier: registry,
		Package:            &pkg,
		Metadata:           items,
	}
	if version != "" {
		body.Version = &version
	}
	resp, err := f.RegistryV2HttpClient().UpdateMetadataWithResponse(context.Background(), params, body)
	if err != nil {
		logrus.Warnf("Failed to publish metadata for %s/%s@%s: %v", registry, pkg, version, err)
		return
	}
	if resp.StatusCode() >= 400 {
		logrus.Warnf("Failed to publish metadata for %s/%s@%s: status %d", registry, pkg, version, resp.StatusCode())
		return
	}
	logrus.Infof("Published %d metadata key(s) to %s/%s@%s", len(items), registry, pkg, version)
}

// applyPostPushAutoMetadata attaches system-generated metadata to an artifact version after a
// successful push via the internal AUTO metadata API. metadataStr uses the same
// "key:value,key2:value2" format as --metadata. It is a no-op when metadataStr is empty or pkg
// is unknown. Failures are logged only when verbose logging is enabled and do not fail the
// push. User-facing messages must not mention the internal endpoint.
func applyPostPushAutoMetadata(f *cmdutils.Factory, metadataStr, registry, pkg, version string) {
	if metadataStr == "" {
		return
	}
	if pkg == "" {
		log.Warn().Msg("Skipping pipeline metadata publish: package name is unknown for this package type")
		return
	}
	items, err := metadata.ParseMetadataString(metadataStr)
	if err != nil {
		log.Warn().Msgf("Failed to parse pipeline metadata for %s/%s@%s: %v", registry, pkg, version, err)
		return
	}
	v3Items := make([]ar_v3.MetadataItemInput, len(items))
	for i, item := range items {
		v3Items[i] = ar_v3.MetadataItemInput{Key: item.Key, Value: item.Value}
	}
	params := &ar_v3.UpsertAutoMetadataV3Params{
		AccountIdentifier: config.Global.AccountID,
	}
	body := ar_v3.UpsertAutoMetadataV3JSONRequestBody{
		RegistryName: registry,
		Package:      &pkg,
		Metadata:     v3Items,
	}
	if version != "" {
		body.Version = &version
	}
	resp, err := f.RegistryV3HttpClient().UpsertAutoMetadataV3WithResponse(context.Background(), params, body)
	if err != nil {
		log.Warn().Msgf("Failed to attach pipeline metadata for %s/%s@%s: %v", registry, pkg, version, err)
		return
	}
	if resp.StatusCode() >= 400 {
		log.Warn().Msgf("Failed to attach pipeline metadata for %s/%s@%s: status %d", registry, pkg, version, resp.StatusCode())
		return
	}
}

// registerAutoMetadataFlag adds a hidden --auto-metadata flag used by plugins to attach
// system-generated metadata after push. It is intentionally omitted from help output.
func registerAutoMetadataFlag(cmd *cobra.Command, autoMetadata *string) {
	cmd.Flags().StringVar(autoMetadata, "auto-metadata", "", "")
	_ = cmd.Flags().MarkHidden("auto-metadata")
}
