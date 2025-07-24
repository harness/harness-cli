package ar

import (
	"fmt"
	commands "github.com/harness/harness-cli/cmd/ar/command"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-go-sdk/harness"
	"github.com/harness/harness-go-sdk/harness/har"
	openapi_client_logging "github.com/harness/harness-openapi-go-client/logging"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func getOpenApiHttpClient(logger *logrus.Logger) *retryablehttp.Client {
	httpClient := retryablehttp.NewClient()
	httpClient.HTTPClient.Transport = openapi_client_logging.NewTransport(harness.SDKName, logger,
		cleanhttp.DefaultPooledClient().Transport)
	httpClient.RetryMax = 10
	return httpClient
}

func getHarClient() *har.APIClient {
	cfg := har.NewConfiguration()
	client := har.NewAPIClient(&har.Configuration{
		AccountId:     config.Global.AccountID,
		BasePath:      config.Global.APIBaseURL + "/gateway/har/api/v1",
		ApiKey:        config.Global.AuthToken,
		UserAgent:     fmt.Sprintf("harness-cli-%s", "0.0.0"),
		HTTPClient:    getOpenApiHttpClient(cfg.Logger),
		DefaultHeader: map[string]string{"X-Api-Key": config.Global.AuthToken},
		DebugLogging:  openapi_client_logging.IsDebugOrHigher(cfg.Logger),
	})
	return client
}

func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "ar",
		Short: "CLI tool for Harness Artifact Registry",
		Long:  `CLI tool for Harness Artifact Registry and migrate artifacts`,
	}

	//client, err := ar.NewClientWithResponses(config.Global.APIBaseURL+"/gateway/har/api/v1", auth.GetXApiKeyOptionAR())
	client := getHarClient()

	rootCmd.AddCommand(
		getMigrateCmd(client),
	)

	rootCmd.AddCommand(
		getGetCommand(
			commands.NewGetRegistryCmd(client),
			commands.NewGetArtifactCmd(client),
			commands.NewGetVersionCmd(client),
			commands.NewFilesVersionCmd(client),
		),
	)

	rootCmd.AddCommand(
		getDeleteCmd(
			commands.NewDeleteRegistryCmd(client),
			commands.NewDeleteArtifactCmd(client),
			commands.NewDeleteVersionCmd(client),
		),
	)

	rootCmd.AddCommand(
		getPushCommand(
			commands.NewPushGenericCmd(client),
			commands.NewPushMavenCmd(client),
		),
	)

	rootCmd.AddCommand(
		getPullCommand(
			commands.NewPullGenericCmd(client),
		),
	)

	return rootCmd
}
