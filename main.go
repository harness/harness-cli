package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"os"
)

func main() {
	fmt.Println("Hello World!")

	//customers := GetCustomers()

	//for _, customer := range customers {
	//we can access the "customer" variable in this approach
	//fmt.Println(customer)
	//}
	globalFlags := []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:  "target-api-key",
			Usage: "`API_KEY` for the target account to authenticate & authorise the migration."}),
	}
	app := &cli.App{
		Name:                 "harness-cli",
		Usage:                "Upgrade Harness CD from Current Gen to Next Gen!",
		EnableBashCompletion: true,
		Suggest:              true,
		Commands: []*cli.Command{
			{
				Name:    "login",
				Aliases: []string{"login"},
				Usage:   "Check for updates and upgrade the CLI",
				Action: func(context *cli.Context) error {
					//return cliWrapper(Update, context)
					fmt.Println("Try to login here.")
					return nil
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
