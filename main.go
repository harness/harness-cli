package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
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
	BaseUrl     string `survey:"https://app.harness.io/"` //TODO : make it environment specific in utils
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
	CheckGithubForReleases()
	globalFlags := []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "base-url",
			Usage:       "provide the `NG_BASE_URL` for self managed platforms",
			Destination: &cliCdRequestData.BaseUrl,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-key",
			Usage:       "`API_KEY` for the target account to authenticate & authorise the user.",
			Destination: &cliCdRequestData.AuthToken,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "account-id",
			Usage:       "`API_KEY` for the target account to authenticate & authorise the user.",
			Destination: &cliCdRequestData.Account,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:  "load",
			Usage: "`FILE` to load flags from.",
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
		Name:                 "harness",
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
				Aliases: []string{"secret-token"},
				Usage:   "Secrets specific commands. eg: apply (create/update), delete",
				Flags: append(globalFlags,
					altsrc.NewStringFlag(&cli.StringFlag{
						Name:  "token",
						Usage: "Specify your PAT",
					}),
					&cli.StringFlag{
						Name:  "file",
						Usage: "`File path for the secret",
					},
				),
				Action: func(context *cli.Context) error {
					fmt.Println("Secrets command.")
					return nil
				},
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx)
					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new secret or Update  an existing one.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "secret-type",
								Usage: "provide the secret type.",
							},
						},
						Action: func(context *cli.Context) error {
							return cliWrapper(applySecret, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a secret.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deleteConnector, context)
						},
					},
				},
			},
			{
				Name:    "service",
				Aliases: []string{"svc"},
				Usage:   "Service specific commands, eg: apply (create/update), delete, list",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Usage: "`YAML` file path for the service",
					},
				},
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx)
					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new service or Update  an existing one.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "cloud-project",
								Usage: "provide the Google Cloud Platform project name.",
							},
							&cli.StringFlag{
								Name:  "cloud-bucket",
								Usage: "provide the Google Cloud Platform bucket name.",
							},
							&cli.StringFlag{
								Name:  "cloud-region",
								Usage: "provide the Google Cloud Platform bucket name.",
							},
						},
						Action: func(context *cli.Context) error {
							return cliWrapper(applyService, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a service.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deleteService, context)
						},
					},
				},
			},
			{
				Name:    "environment",
				Aliases: []string{"env"},
				Usage:   "Environment specific commands, eg: apply (create/update), delete, list",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Usage: "`YAML` file path for the environment",
					},
				},
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx)
					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new environment or Update  an existing one.",
						Action: func(context *cli.Context) error {
							return cliWrapper(applyEnvironment, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete an environment.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deleteEnvironment, context)
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
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx)
					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new connector or Update  an existing one.",
						Flags: []cli.Flag{

							&cli.StringFlag{
								Name:  "delegate-name",
								Usage: "delegate name for the k8s connector",
							},
							&cli.StringFlag{
								Name:  "git-user",
								Usage: "git username for the github connector",
							},
							&cli.StringFlag{
								Name:  "aws-cross-account-role-arn",
								Usage: "cross account role arn for the aws connector",
							},
							&cli.StringFlag{
								Name:  "aws-access-key",
								Usage: "access key for the aws connector",
							},
							&cli.StringFlag{
								Name:  "aws-secret-Key",
								Usage: "access secret for the aws connector",
							},
							&cli.StringFlag{
								Name:  "cloud-region",
								Usage: "region for the cloud connector",
							},
						},
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
				Name:    "infrastructure",
				Aliases: []string{"infra"},
				Usage:   "Infrastructure specific commands, eg: apply (create/update), delete, list",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Usage: "`YAML` file path for the infrastructure",
					},
				},
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx)
					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new infrastructure or Update  an existing one.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "cloud-project",
								Usage: "provide the Google Cloud Platform project name. ",
							},
							&cli.StringFlag{
								Name:  "cloud-region",
								Usage: "provide the Cloud Platform region name. For eg; 1.Creating GCP pipeline then provide gcp-region name, 2.Creating AWS based pipeline then provide aws-region name",
							},
						},
						Action: func(context *cli.Context) error {
							return cliWrapper(applyInfraDefinition, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a infrastructure.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deleteInfraDefinition, context)
						},
					},
				},
			},
			{
				Name:    "pipeline",
				Aliases: []string{"pipeline"},
				Usage:   "Pipeline specific commands, eg: apply (create/update), delete, list",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Usage: "`YAML` file path for the pipeline",
					},
				},
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx)
					return nil
				},
				Subcommands: []*cli.Command{
					{
						Name:  "apply",
						Usage: "Create a new pipeline or Update  an existing one.",
						Action: func(context *cli.Context) error {
							return cliWrapper(applyPipeline, context)
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a pipeline.",
						Action: func(context *cli.Context) error {
							return cliWrapper(deletePipeline, context)
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
					return cliWrapper(Login, context)
				},
				Before: func(ctx *cli.Context) error {
					hydrateCredsFromPersistence(ctx, true)
					return nil
				},
			},
			{
				Name:    "account",
				Aliases: []string{"acc"},
				Usage:   "Fetch Account details",
				Flags:   globalFlags,
				Action: func(context *cli.Context) error {
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

func beforeAction(globalFlags []cli.Flag) {
	altsrc.InitInputSourceWithContext(globalFlags, altsrc.NewYamlSourceFromFlagFunc("load"))
}
