package command

import (
	"io"
	"time"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
	"github.com/harness/harness-cli/config"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// session bundles the pieces every load-test command needs: a configured API
// client and a printer honouring the resolved --format.
type session struct {
	client  *api.Client
	printer *resultPrinter
}

func newSession(cmd *cobra.Command) (*session, error) {
	format, err := parseFormat(config.Global.Format)
	if err != nil {
		return nil, err
	}

	return &session{
		client:  newClient(),
		printer: &resultPrinter{format: format, out: cmd.OutOrStdout()},
	}, nil
}

// newClient builds the API client from the resolved global configuration, as
// iacm does, so registering the command costs the shared factory nothing.
func newClient() *api.Client {
	// Scope goes over as query parameters, not in the path as the registry
	// APIs do. A zero timeout means unset, not no timeout.
	return api.NewClient(api.Config{
		Server:  config.Global.APIBaseURL,
		Scope:   resolveScope(),
		Timeout: time.Duration(config.Global.TimeoutSeconds) * time.Second,
	})
}

// resultPrinter takes a projection the caller has already built, for the cells
// the shared printer's reflection path cannot derive.
type resultPrinter struct {
	format string
	out    io.Writer
}

// Print uses the projection for tables and the full response for json/yaml. A
// detail view passes nil and falls through to YAML rather than one wide row.
// The full response goes through readable first, so a script arrives as script
// rather than as base64. Doing that here rather than at each call site means a
// command added later inherits it without having to remember.
func (p *resultPrinter) Print(data any, table *Table) error {
	switch p.format {
	case formatJSON:
		return renderJSON(readable(data), p.out)
	case formatYAML:
		return renderYAML(readable(data), p.out)
	default:
		if table == nil {
			return renderYAML(readable(data), p.out)
		}
		return renderTable(*table, p.out)
	}
}

// registerConfigFlag adds the load-test definition file flag.
func registerConfigFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().String(FlagConfig, "", usage)
}

// changedFlags reports which flags the user actually typed, so unset flags
// never clobber values that came from a config file.
func changedFlags(cmd *cobra.Command) map[string]bool {
	set := map[string]bool{}
	cmd.Flags().Visit(func(flag *pflag.Flag) { set[flag.Name] = true })
	return set
}

// resolveScope is for the endpoints that want the scope in the body as well as
// the query string.
func resolveScope() api.Scope {
	return api.Scope{
		AccountID: config.Global.AccountID,
		OrgID:     config.Global.OrgID,
		ProjectID: config.Global.ProjectID,
	}
}

// scopeDescription names the current scope for an error message, since a "not
// found" is most often the wrong project rather than a missing object.
func scopeDescription() string {
	scope := resolveScope()
	if scope.OrgID == "" || scope.ProjectID == "" {
		return "the current project"
	}
	return "project " + scope.ProjectID + " of organization " + scope.OrgID
}
