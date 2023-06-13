package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
)

func Login(ctx *cli.Context) (err error) {
	PromptAccountDetails(ctx)
	fmt.Println("Account is=", cliCdRequestData.Account)
	getAccountDetails(ctx)
	saveCredentials()
	return nil
}
