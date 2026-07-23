package upload

import (
	"fmt"

	ar "github.com/harness/harness-cli/internal/api/ar"
	pkgclient "github.com/harness/harness-cli/internal/api/ar_pkg"
)

// UploaderConfig holds all parameters needed to construct any Pusher implementation.
type UploaderConfig struct {
	SrcPattern     string
	PackageVersion string
	DryRun         bool
	Flatten        bool
	Include        []string
	Exclude        []string
	PkgClient      *pkgclient.ClientWithResponses
}

// getPusherInstance returns the appropriate Pusher for the given registry PackageType.
// RAW    → RawUploader
// GENERIC → GenericUploader
// all other types → error (upload not supported) can be implemented later
func getPusherInstance(pkgType ar.PackageType, cfg UploaderConfig) (Pusher, error) {
	switch pkgType {
	case "RAW":
		return &RawUploader{
			SrcPattern: cfg.SrcPattern,
			DryRun:     cfg.DryRun,
			Flatten:    cfg.Flatten,
			Include:    cfg.Include,
			Exclude:    cfg.Exclude,
			PkgClient:  cfg.PkgClient,
		}, nil
	case "GENERIC":
		return &GenericUploader{
			SrcPattern: cfg.SrcPattern,
			Version:    cfg.PackageVersion,
			DryRun:     cfg.DryRun,
			PkgClient:  cfg.PkgClient,
		}, nil
	default:
		return nil, fmt.Errorf("upload command is not supported for package type %q", pkgType)
	}
}
