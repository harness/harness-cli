package command

import (
	"context"
	"fmt"
	"os"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/style"
	"github.com/harness/harness-cli/internal/tui"
	client2 "github.com/harness/harness-cli/util/client"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func NewDeleteRegistryCmd(c *cmdutils.Factory) *cobra.Command {
	var (
		name  string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete registry",
		Long:  "Delete a registry from Harness Artifact Registry",
		Example: `  # Delete a registry (with confirmation in a terminal)
  hc registry delete my-registry

  # Skip confirmation in scripts
  hc registry delete my-registry --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name = args[0]
			if len(name) == 0 {
				return fmt.Errorf("must specify registry name")
			}

			// Interactive confirmation when running in a TTY (unless --force)
			if !force && term.IsTerminal(int(os.Stdout.Fd())) {
				confirmed, err := tui.ConfirmDeletion("registry", name)
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println(style.DimText.Render("Deletion cancelled."))
					return nil
				}
			}

			client := c.RegistryHttpClient()

			response, err := client.DeleteRegistryWithResponse(context.Background(),
				client2.GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID, name))
			if err != nil {
				return err
			}
			if response.JSON200 != nil {
				if term.IsTerminal(int(os.Stdout.Fd())) {
					fmt.Println(style.Success.Render("✓ Deleted registry " + name))
				} else {
					log.Info().Msgf("Deleted registry %s", name)
				}
			} else {
				log.Error().Msgf("failed to delete registry %s %s", name, string(response.Body))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")

	return cmd
}
