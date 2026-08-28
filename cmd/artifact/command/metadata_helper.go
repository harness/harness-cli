package command

import (
	"context"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/util/metadata"
	"github.com/sirupsen/logrus"
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
