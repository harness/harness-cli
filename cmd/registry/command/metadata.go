package command

import (
	"github.com/harness/harness-cli/cmd/cmdutils"

	"github.com/spf13/cobra"
)

func NewMetadataCmd(f *cmdutils.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "Manage registry metadata",
		Long:  "Commands to manage metadata on registries in Harness Artifact Registry",
	}

	cmd.AddCommand(NewMetadataGetCmd(f))
	cmd.AddCommand(NewMetadataSetCmd(f))
	cmd.AddCommand(NewMetadataDeleteCmd(f))

	return cmd
}
