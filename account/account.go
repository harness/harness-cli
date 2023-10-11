package account

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	"harness/utils"
)

func GetAccountDetails(ctx *cli.Context) error {
	// Getting the account details
	var baseURL = utils.GetNGBaseURL(ctx)
	accountsEndpoint := defaults.ACCOUNTS_ENDPOINT + CliCdRequestData.Account
	url := utils.GetUrlWithQueryParams("", baseURL, accountsEndpoint, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
	})
	resp, err := client.Get(url, CliCdRequestData.AuthToken)
	if err != nil {
		fmt.Printf("Response status: %s \n", resp.Status)
		fmt.Printf("Response code: %s \n", resp.Code)
		fmt.Printf("Response resource: %s \n", resp.Resource)
		fmt.Printf("Response messages: %s \n", resp.Messages)
		return nil
	}

	return nil
}
