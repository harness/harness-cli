package cmdutils

import (
	"context"
	"fmt"
	"strings"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/httpclient"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// ShouldDerivePkgURL reports whether the package base URL should be derived
// from the API base URL via the HAR /system/info endpoint. Derivation only
// kicks in for the explicit-credential flow: the caller passed --api-url and
// no package URL is already configured (neither via --pkg-url nor via the
// saved auth.json login config).
func ShouldDerivePkgURL(apiURLFlagSet bool, currentPkgURL string) bool {
	return apiURLFlagSet && strings.TrimSpace(currentPkgURL) == ""
}

// ResolvePkgURL settles config.Global.Registry.PkgURL for artifact push/pull
// commands before any bytes are streamed. Precedence:
//
//  1. explicit --pkg-url flag (normalized via util.GetPkgUrl);
//  2. package URL already configured (saved auth.json login) — left untouched;
//  3. derived from --api-url via /system/info, the same resolution
//     `hc auth login` performs (see cmd/auth/login.go).
//
// It returns an error when the endpoint is still empty so every push type
// fails fast instead of streaming bytes into a 404 from a wrong package root.
func ResolvePkgURL(cmd *cobra.Command, pkgURLFlag string) error {
	switch {
	case pkgURLFlag != "":
		config.Global.Registry.PkgURL = util.GetPkgUrl(pkgURLFlag)
	case ShouldDerivePkgURL(apiURLFlagChanged(cmd), config.Global.Registry.PkgURL):
		derived, err := derivePkgURLFromAPI(commandContext(cmd))
		if err != nil {
			return fmt.Errorf("could not derive package URL from --api-url %q: %w (pass --pkg-url to set it explicitly)",
				config.Global.APIBaseURL, err)
		}
		config.Global.Registry.PkgURL = derived
		log.Info().Msgf("Derived package registry URL from --api-url: %s", derived)
	}

	if strings.TrimSpace(config.Global.Registry.PkgURL) == "" {
		return fmt.Errorf("pkg-url must be set: no package registry URL configured — " +
			"pass --pkg-url, set --api-url so it can be derived, or run 'hc auth login'")
	}
	log.Debug().Msgf("Using package registry endpoint: %s/pkg/%s",
		config.Global.Registry.PkgURL, config.Global.AccountID)
	return nil
}

// apiURLFlagChanged reports whether --api-url was explicitly set on the
// command line. The flag is a persistent root flag bound to
// config.Global.APIBaseURL; pflag returns false for flags the command does
// not know (e.g. in unit tests), which safely disables derivation there.
func apiURLFlagChanged(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.Flags().Changed("api-url")
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil {
		if ctx := cmd.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// derivePkgURLFromAPI resolves the package registry base URL for the current
// API base URL by calling the HAR /system/info endpoint — the same approach
// as `hc auth login` (cmd/auth/login.go fetchRegistryURL). config.Global must
// already carry the API base URL, token and account ID (flag binding does
// this before PreRunE runs).
func derivePkgURLFromAPI(ctx context.Context) (string, error) {
	if strings.TrimSpace(config.Global.APIBaseURL) == "" {
		return "", fmt.Errorf("--api-url is empty")
	}
	if strings.TrimSpace(config.Global.AccountID) == "" {
		return "", fmt.Errorf("account ID is empty")
	}

	client, err := ar_v3.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v3",
		ar_v3.WithHTTPClient(httpclient.NewRetryClientWithoutProgress()),
		auth.GetXApiKeyOptionARV3())
	if err != nil {
		return "", fmt.Errorf("failed to create registry client: %w", err)
	}

	resp, err := client.GetSystemInfoWithResponse(ctx, &ar_v3.GetSystemInfoParams{
		AccountIdentifier: config.Global.AccountID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to call /system/info: %w", err)
	}

	if resp.JSON200 != nil {
		if data, ok := (*resp.JSON200)["data"].(map[string]interface{}); ok {
			if registryURL, ok := data["registryUrl"].(string); ok && registryURL != "" {
				return registryURL, nil
			}
		}
	}

	return "", fmt.Errorf("failed to extract registryUrl from /system/info response")
}
