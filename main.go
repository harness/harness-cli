package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"os"
)

type cliFnWrapper func(ctx *cli.Context) error

var cliCdReq = struct {
	AuthToken   string `survey:"authToken"`
	AuthType    string `survey:"authType"`
	Account     string `survey:"account"`
	OrgName     string `survey:"default"`
	ProjectName string `survey:"default"`
	Debug       bool   `survey:"debug"`
	Json        bool   `survey:"json"`
	BaseUrl     string `survey:"https://app.harness.io/gateway/ng"` //TODO : make it environment specific in utils
}{}

func main() {
	fmt.Println("Welcome to Harness CLI!")

	globalFlags := []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-key",
			Usage:       "`API_KEY` for the target account to authenticate & authorise the migration.",
			Destination: &cliCdReq.AuthToken,
		}),
	}
	app := &cli.App{
		Name:                 "harness-cli",
		Usage:                "Harness CLI Onboarding tool!",
		EnableBashCompletion: true,
		Suggest:              true,
		Commands: []*cli.Command{
			{
				Name:    "login",
				Aliases: []string{"login"},
				Usage:   "Login with account identifier and api key.",
				Flags:   globalFlags,
				Action: func(context *cli.Context) error {
					fmt.Println("Try to login here.")
					return cliWrapper(Login, context)
					//return nil
				},
			},
		},
		Before: altsrc.InitInputSourceWithContext(globalFlags, altsrc.NewYamlSourceFromFlagFunc("load")),
		Flags:  globalFlags,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}

func cliWrapper(fn cliFnWrapper, ctx *cli.Context) error {
	if cliCdReq.Debug {
		log.SetLevel(log.DebugLevel)
	}
	if cliCdReq.Json {
		log.SetFormatter(&log.JSONFormatter{})
	}
	return fn(ctx)
}
