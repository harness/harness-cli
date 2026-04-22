package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	mvnclient "github.com/harness/harness-cli/cmd/pkgmgr/maven"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewMavenInstallCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedMavenCmd(f, "install")
}

func NewMavenPackageCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedMavenCmd(f, "package")
}

// newWrappedMavenCmd creates a cobra command that wraps a native maven command
// with firewall integration.
func newWrappedMavenCmd(f *cmdutils.Factory, mvnCommand string) *cobra.Command {
	var registryName string

	cmd := &cobra.Command{
		Use:   mvnCommand + " [mvn-args...]",
		Short: fmt.Sprintf("Run mvn %s with firewall support", mvnCommand),
		Long: fmt.Sprintf(`Wrap mvn %s with firewall integration.

Runs the native mvn %s. On 403 errors, displays detailed firewall violations.`, mvnCommand, mvnCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			var mvnArgs []string
			for i := 0; i < len(args); i++ {
				switch {
				case args[i] == "--registry" && i+1 < len(args):
					registryName = args[i+1]
					i++
				case strings.HasPrefix(args[i], "--registry="):
					registryName = strings.TrimPrefix(args[i], "--registry=")
				case args[i] == "-v" || args[i] == "--verbose":
					logWriter := zerolog.ConsoleWriter{
						Out:        os.Stderr,
						TimeFormat: time.RFC3339,
						NoColor:    false,
					}
					log.Logger = log.Output(logWriter)
				default:
					mvnArgs = append(mvnArgs, args[i])
				}
			}

			client := mvnclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, mvnCommand, mvnArgs, registryName, progress)
		},
	}

	return cmd
}
