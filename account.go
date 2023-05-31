package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
)

func getAccountDetails(ctx *cli.Context) error {
	// Getting the account details
	url := GetUrlWithQueryParams("", "", "accounts", map[string]string{
		"accountIdentifier": cliCdReq.Account,
	})
	resp, err := Get(url, cliCdReq.AuthToken)

	if err == nil {
		fmt.Printf("Response status: %s \n", resp.Status)
		fmt.Printf("Response code: %s \n", resp.Code)
		fmt.Printf("Response resource: %s \n", resp.Resource)
		fmt.Printf("Response messages: %s \n", resp.Messages)
		printJson(resp.Data)
		return nil
	}

	return nil
}
