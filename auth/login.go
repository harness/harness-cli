package auth

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli/v2"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	"harness/types"
	. "harness/utils"
	"os"
)

func Login(ctx *cli.Context) (err error) {

	fmt.Println("Welcome to Harness CLI!")
	PromptAccountDetails(ctx)
	SaveCredentials(ctx, false) // Suppress child showWelcome funcn while login in progress
	loginError := GetAccountDetails(ctx)

	if loginError != nil {
		telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.LOGIN_FAILED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
			"accountId": CliCdRequestData.Account,
			"userId":    CliCdRequestData.UserId,
		})
		return nil
	}
	GetUserDetails(ctx)
	SaveCredentials(ctx, true) // Call child showWelcome funcn if login succeeds
	telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.LOGIN_SUCCESS, UserId: CliCdRequestData.UserId}, map[string]interface{}{
		"accountId": CliCdRequestData.Account,
		"userId":    CliCdRequestData.UserId,
	})

	return nil
}

func HydrateCredsFromPersistence(params ...interface{}) {
	c := params[0].(*cli.Context)
	var hydrateOnlyURL = false

	if len(params) > 1 {
		if value, ok := params[1].(bool); ok {
			hydrateOnlyURL = value
		}
	}
	if CliCdRequestData.AuthToken != "" && CliCdRequestData.Account != "" && !hydrateOnlyURL {
		return
	}

	exactFilePath := GetUserHomePath() + "/" + defaults.SECRETS_STORE_PATH
	credsJson, err := os.ReadFile(exactFilePath)
	if err != nil {
		fmt.Println("Error reading creds file:", err)
		return
	}
	var secretstore types.SecretStore
	err = json.Unmarshal(credsJson, &secretstore)
	if err != nil {
		fmt.Println("Error:", err)
		Login(c)
		return
	}
	if hydrateOnlyURL {
		baseURL := c.String("base-url")
		if baseURL == "" {
			CliCdRequestData.BaseUrl = secretstore.BaseURL
		} else {
			CliCdRequestData.BaseUrl = baseURL
		}
	} else {
		CliCdRequestData.AuthToken = secretstore.ApiKey
		CliCdRequestData.Account = secretstore.AccountId
		CliCdRequestData.BaseUrl = secretstore.BaseURL
		CliCdRequestData.UserId = secretstore.UserId
	}
	return
}
