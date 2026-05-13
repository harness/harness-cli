package command

import (
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	pipclient "github.com/harness/harness-cli/cmd/pkgmgr/pip"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewPipInstallCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedPipCmd(f, "install")
}

func newWrappedPipCmd(f *cmdutils.Factory, pipCommand string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   pipCommand + " [pip-args...]",
		Short: fmt.Sprintf("Run pip %s with firewall support", pipCommand),
		Long: fmt.Sprintf(`Wrap pip %s with firewall integration.

Runs the native pip %s. On 403 errors, displays detailed firewall violations.`, pipCommand, pipCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed := pkgmgr.ParseWrappedArgs(args)
			progress := p.NewConsoleReporter()
			client := pipclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, pipCommand, parsed.NativeArgs, parsed.RegistryName, progress)
		},
	}

	return cmd
}
