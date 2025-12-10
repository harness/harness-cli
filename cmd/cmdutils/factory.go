package cmdutils

import (
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar"
	"github.com/harness/harness-cli/util/common/auth"

	"github.com/rs/zerolog/log"
)

type Factory struct {
	// This will be overtaken by main harness-go-sdk client
	RegistryHttpClient func() *ar.ClientWithResponses
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
	}
}
