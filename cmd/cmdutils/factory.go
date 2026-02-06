package cmdutils

import (
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/internal/api/ar_v2"
	"github.com/harness/harness-cli/internal/api/ar_v3"
	"github.com/harness/harness-cli/util/common/auth"

	"github.com/rs/zerolog/log"
)

type Factory struct {
	// This will be overtaken by main harness-go-sdk client
	RegistryHttpClient   func() *ar.ClientWithResponses
	RegistryV2HttpClient func() *ar_v2.ClientWithResponses
	RegistryV3HttpClient func() *ar_v3.ClientWithResponses
}

func NewFactory() *Factory {
	return &Factory{
		RegistryHttpClient: func() *ar.ClientWithResponses {
			client, err := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1",
				auth.GetXApiKeyOptionAR())
			if err != nil {
				log.Fatal().Msgf("Error creating client: %v", err)
			}
			return client
		},
		RegistryV2HttpClient: func() *ar_v2.ClientWithResponses {
			client, err := ar_v2.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v2",
				auth.GetXApiKeyOptionARV2())
			if err != nil {
				log.Fatal().Msgf("Error creating client: %v", err)
			}
			return client
		},
		RegistryV3HttpClient: func() *ar_v3.ClientWithResponses {
			client, err := ar_v3.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v3",
				auth.GetXApiKeyOptionARV3())
			if err != nil {
				log.Fatal().Msgf("Error creating client: %v", err)
			}
			return client
		},
	}
}
