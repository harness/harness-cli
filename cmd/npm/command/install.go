package command

import (
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	npmclient "github.com/harness/harness-cli/cmd/pkgmgr/npm"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewNpmInstallCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedNpmCmd(f, "install")
}

func NewNpmCiCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedNpmCmd(f, "ci")
}

func newWrappedNpmCmd(f *cmdutils.Factory, npmCommand string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   npmCommand + " [npm-args...]",
		Short: fmt.Sprintf("Run npm %s with firewall support", npmCommand),
		Long: fmt.Sprintf(`Wrap npm %s with firewall integration.

Runs the native npm %s. On 403 errors, displays detailed firewall violations.`, npmCommand, npmCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed := pkgmgr.ParseWrappedArgs(args)
			progress := p.NewConsoleReporter()
			client := npmclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, npmCommand, parsed.NativeArgs, parsed.RegistryName, progress)
		},
	}

	return cmd
}
