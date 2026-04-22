package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	pipclient "github.com/harness/harness-cli/cmd/pkgmgr/python"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewPipInstallCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedPipCmd(f, "install")
}

// newWrappedPipCmd creates a cobra command that wraps a native pip command
// with firewall integration.
func newWrappedPipCmd(f *cmdutils.Factory, pipCommand string) *cobra.Command {
	var registryName string

	cmd := &cobra.Command{
		Use:   pipCommand + " [pip-args...]",
		Short: fmt.Sprintf("Run pip %s with firewall support", pipCommand),
		Long: fmt.Sprintf(`Wrap pip %s with firewall integration.

Runs the native pip %s. On 403 errors, displays detailed firewall violations.`, pipCommand, pipCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			var pipArgs []string
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
					pipArgs = append(pipArgs, args[i])
				}
			}

			client := pipclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, pipCommand, pipArgs, registryName, progress)
		},
	}

	return cmd
}
