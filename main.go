package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"os"
)

var Version = "development"

type cliFnWrapper func(ctx *cli.Context) error

var cliCdRequestData = struct {
	AuthToken   string `survey:"authToken"`
	AuthType    string `survey:"authType"`
	Account     string `survey:"account"`
	OrgName     string `survey:"default"`
	ProjectName string `survey:"default"`
	Debug       bool   `survey:"debug"`
	Json        bool   `survey:"json"`
	BaseUrl     string `survey:"https://app.harness.io/gateway/ng"` //TODO : make it environment specific in utils
}{}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	// Output to stdout instead of the default stderr
	log.SetOutput(os.Stdout)
	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
	cli.VersionPrinter = func(cCtx *cli.Context) {
		fmt.Println(cCtx.App.Version)
	}
}

func main() {
	fmt.Println("Welcome to Harness CLI!")
	CheckGithubForReleases()
	globalFlags := []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "base-url",
			Usage:       "provide the `BASE_URL` for self managed platforms",
			Destination: &cliCdRequestData.BaseUrl,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-key",
			Usage:       "`API_KEY` for the target account to authenticate & authorise the user.",
			Destination: &cliCdRequestData.AuthToken,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "load",
			Usage:       "`FILE` to load flags from.",
			Destination: &cliCdRequestData.AuthToken,
		}),

		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:        "debug",
			Usage:       "prints debug level logs",
			Destination: &cliCdRequestData.Debug,
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:        "json",
			Usage:       "log as JSON instead of standard ASCII formatter",
			Destination: &cliCdRequestData.Json,
		}),
	}
	app := &cli.App{
		Name:                 "harness-cli",
		Version:              Version,
		Usage:                "Setup Harness CD & GitOps in a few commands.",
		EnableBashCompletion: true,
		Suggest:              true,
		Commands: []*cli.Command{
			{
				Name:    "update",
				Aliases: []string{"upgrade"},
				Usage:   "Check for updates and upgrade the CLI",
				Action: func(context *cli.Context) error {
					return cliWrapper(Update, context)
				},
			},
			{
				Name:    "secret",
				Aliases: []string{"secret"},
				Usage:   "Secrets apply (create or update), delete",
				Flags:   globalFlags,
				Action: func(context *cli.Context) error {
					fmt.Println("Secrets command.")
					return nil
				},
			},
			{
				Name:    "service",
				Aliases: []string{"svc"},
				Usage:   "Service specific commands, eg: apply (create/update), delete, list",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Usage: "`YAML` file path for the connector",
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new connector or Update  an existing one.",
						Action: func(context *cli.Context) error {
							return cliWrapper(applyConnector, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a connector.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deleteConnector, context)
						},
					},
				},
			},
			{
				Name:    "connector",
				Aliases: []string{"conn"},
				Usage:   "Connector specific commands, eg: apply (create/update), delete, list",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Usage: "`YAML` file path for the connector",
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new connector or Update  an existing one.",
						Action: func(context *cli.Context) error {
							return cliWrapper(applyConnector, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a connector.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deleteConnector, context)
						},
					},
				},
			},
			{
				Name:    "login",
				Aliases: []string{"login"},
				Usage:   "Login with account identifier and api key.",
				Flags:   globalFlags,
				Action: func(context *cli.Context) error {
					fmt.Println("Trying to login here.")
					return cliWrapper(Login, context)
				},
			},
			{
				Name:    "account",
				Aliases: []string{"acc"},
				Usage:   "Fetch Account details",
				Flags:   globalFlags,
				Action: func(context *cli.Context) error {
					fmt.Println("Trying to login here.")
					return cliWrapper(getAccountDetails, context)
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
	if cliCdRequestData.Debug {
		log.SetLevel(log.DebugLevel)
	}
	if cliCdRequestData.Json {
		log.SetFormatter(&log.JSONFormatter{})
	}
	return fn(ctx)
}
