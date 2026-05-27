package cmdutils

import (
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/auth"
	"github.com/harness/harness-cli/util/common/httpclient"
	"github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog/log"
)

type Factory struct {
	// This will be overtaken by main harness-go-sdk client
	RegistryHttpClient   func() *ar.ClientWithResponses
	RegistryV2HttpClient func() *ar_v2.ClientWithResponses
	RegistryV3HttpClient func() *ar_v3.ClientWithResponses
	PkgHttpClient        func() *ar_pkg.ClientWithResponses
}

func (f *Factory) NewRegistryV3HttpClientWithURL(url string) (*ar_v3.ClientWithResponses, error) {
	return ar_v3.NewClientWithResponses(url,
		ar_v3.WithHTTPClient(httpclient.NewRetryClientWithoutProgress()),
		auth.GetXApiKeyOptionARV3())
}

// PkgHttpClientWithProgress creates a package client with retry and progress reporting support
func (f *Factory) PkgHttpClientWithProgress(p progress.Reporter, fileSize int64, saveFilename string) *ar_pkg.ClientWithResponses {
	client, err := ar_pkg.NewClientWithResponses(config.Global.Registry.PkgURL,
		ar_pkg.WithHTTPClient(httpclient.NewRetryClientWithProgress(p, fileSize, saveFilename)),
		auth.GetAuthOptionARPKG())
	if err != nil {
		log.Fatal().Msgf("Error creating pkg client: %v", err)
	}
	return client
}

func NewFactory() *Factory {
	return &Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses {
			client, err := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1",
				ar.WithHTTPClient(httpclient.NewRetryClientWithoutProgress()),
				auth.GetXApiKeyOptionAR())
			if err != nil {
				log.Fatal().Msgf("Error creating client: %v", err)
			}
			return client
		},
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			client, err := ar_v2.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v2",
				ar_v2.WithHTTPClient(httpclient.NewRetryClientWithoutProgress()),
				auth.GetXApiKeyOptionARV2())
			if err != nil {
				log.Fatal().Msgf("Error creating client: %v", err)
			}
			return client
		},
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
			client, err := ar_v3.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v3",
				ar_v3.WithHTTPClient(httpclient.NewRetryClientWithoutProgress()),
				auth.GetXApiKeyOptionARV3())
			if err != nil {
				log.Fatal().Msgf("Error creating client: %v", err)
			}
			return client
		},
		PkgHttpClient: func() *ar_pkg.ClientWithResponses {
			client, err := ar_pkg.NewClientWithResponses(config.Global.Registry.PkgURL,
				ar_pkg.WithHTTPClient(httpclient.NewRetryClientWithoutProgress()),
				auth.GetAuthOptionARPKG())
			if err != nil {
				log.Fatal().Msgf("Error creating pkg client: %v", err)
			}
			return client
		},
	}
}
