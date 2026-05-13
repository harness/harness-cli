package command

import (
	"fmt"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	mavenclient "github.com/harness/harness-cli/cmd/pkgmgr/maven"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/spf13/cobra"
)

func NewMvnInstallCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedMvnCmd(f, "install")
}

func newWrappedMvnCmd(f *cmdutils.Factory, mvnCommand string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   mvnCommand + " [mvn-args...]",
		Short: fmt.Sprintf("Run mvn %s with firewall support", mvnCommand),
		Long: fmt.Sprintf(`Wrap mvn %s with firewall integration.

Runs the native mvn %s. On 403 errors, displays detailed firewall violations.`, mvnCommand, mvnCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed := pkgmgr.ParseWrappedArgs(args)
			progress := p.NewConsoleReporter()
			client := mavenclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, mvnCommand, parsed.NativeArgs, parsed.RegistryName, progress)
		},
	}

	return cmd
}
