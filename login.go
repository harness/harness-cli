package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func Login(ctx *cli.Context) (err error) {
	fmt.Println("Welcome to Harness CLI!")
	PromptAccountDetails(ctx)
	getAccountDetails(ctx)
	saveCredentials()
	return nil
}
