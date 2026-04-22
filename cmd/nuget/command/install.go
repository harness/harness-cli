package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	nugetclient "github.com/harness/harness-cli/cmd/pkgmgr/nuget"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewDotnetRestoreCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedDotnetCmd(f, "restore")
}

func NewDotnetBuildCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedDotnetCmd(f, "build")
}

// newWrappedDotnetCmd creates a cobra command that wraps a native dotnet command
// with firewall integration.
func newWrappedDotnetCmd(f *cmdutils.Factory, dotnetCommand string) *cobra.Command {
	var registryName string

	cmd := &cobra.Command{
		Use:   dotnetCommand + " [dotnet-args...]",
		Short: fmt.Sprintf("Run dotnet %s with firewall support", dotnetCommand),
		Long: fmt.Sprintf(`Wrap dotnet %s with firewall integration.

Runs the native dotnet %s. On 403 errors, displays detailed firewall violations.`, dotnetCommand, dotnetCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			var dotnetArgs []string
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
					dotnetArgs = append(dotnetArgs, args[i])
				}
			}

			client := nugetclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, dotnetCommand, dotnetArgs, registryName, progress)
		},
	}

	return cmd
}
