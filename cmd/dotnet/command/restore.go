package command

import (
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	nugetclient "github.com/harness/harness-cli/cmd/pkgmgr/nuget"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewDotnetRestoreCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedDotnetCmd(f, "restore")
}

func newWrappedDotnetCmd(f *cmdutils.Factory, dotnetCommand string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   dotnetCommand + " [dotnet-args...]",
		Short: fmt.Sprintf("Run dotnet %s with firewall support", dotnetCommand),
		Long: fmt.Sprintf(`Wrap dotnet %s with firewall integration.

Runs the native dotnet %s. On 403 errors, displays detailed firewall violations.`, dotnetCommand, dotnetCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed := pkgmgr.ParseWrappedArgs(args)
			progress := p.NewConsoleReporter()
			client := nugetclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, dotnetCommand, parsed.NativeArgs, parsed.RegistryName, progress)
		},
	}

	return cmd
}
