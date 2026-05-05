package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/cmd/pkgmgr"
	npmclient "github.com/harness/harness-cli/cmd/pkgmgr/npm"
	p "github.com/harness/harness-cli/util/common/progress"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewNpmInstallCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedNpmCmd(f, "install")
}

func NewNpmCiCmd(f *cmdutils.Factory) *cobra.Command {
	return newWrappedNpmCmd(f, "ci")
}

// newWrappedNpmCmd creates a cobra command that wraps a native npm command
// (install or ci) with firewall integration.
func newWrappedNpmCmd(f *cmdutils.Factory, npmCommand string) *cobra.Command {
	var registryName string

	cmd := &cobra.Command{
		Use:   npmCommand + " [npm-args...]",
		Short: fmt.Sprintf("Run npm %s with firewall support", npmCommand),
		Long: fmt.Sprintf(`Wrap npm %s with firewall integration.

Runs the native npm %s. On 403 errors, displays detailed firewall violations.`, npmCommand, npmCommand),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := p.NewConsoleReporter()

			// Extract our flags from args; pass the rest through to npm.
			// DisableFlagParsing is true, so cobra won't handle persistent
			// flags like -v/--verbose — we must do it ourselves.
			var npmArgs []string
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
					npmArgs = append(npmArgs, args[i])
				}
			}

			client := npmclient.NewClient()
			return pkgmgr.ExecuteWithFirewall(client, f, npmCommand, npmArgs, registryName, progress)
		},
	}

	return cmd
}
