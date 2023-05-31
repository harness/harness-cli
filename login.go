package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
)

func Login(ctx *cli.Context) (err error) {
	apiKey := ctx.String("api-key")
	fmt.Println("API-Key is", apiKey)
	PromptEnvDetails()

	fmt.Println("Account is=", cliCdReq.Account)
	getAccountDetails(ctx)
	return nil
}
