package pr

import (
	"fmt"
	"strconv"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/spf13/cobra"
)

type checksExitError struct {
	code int
}

func (e *checksExitError) Error() string {
	if e.code == 1 {
		return "some checks have failed"
	}
	return "some checks are still running"
}

func (e *checksExitError) ExitCode() int {
	return e.code
}

func newChecksCmd(f *cmdutils.Factory) *cobra.Command {
	var (
		wait       bool
		timeout    int
		repo       string
		jsonFields string
	)

	cmd := &cobra.Command{
		Use:   "checks <number>",
		Short: "View CI check status on a pull request",
		Long:  "Show pipeline/check status for a pull request. Exit code: 0=all pass, 1=any failed, 2=still running.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid PR number %q: %w", args[0], err)
			}

			repoRef, err := resolveRepoRef(repo)
			if err != nil {
				return err
			}

			client := f.CodeClient()

			deadline := time.Now().Add(time.Duration(timeout) * time.Second)

			for {
				checks, err := client.GetPullRequestChecks(repoRef, number)
				if err != nil {
					return err
				}

				anyFailed := false
				anyRunning := false

				for _, c := range checks.Checks {
					switch c.Detail.Status {
					case "success":
						// ok
					case "failure", "error":
						anyFailed = true
					case "pending", "running":
						anyRunning = true
					default:
						anyFailed = true
					}
				}

				if !wait || !anyRunning || time.Now().After(deadline) {
					if jsonFields != "" {
						return printJSON(checks.Checks, jsonFields)
					}

					err = printer.Print(checks.Checks, 0, 0, int64(len(checks.Checks)), false, [][]string{
						{"check.identifier", "Name"},
						{"check.status", "Status"},
						{"required", "Required"},
						{"check.link", "Link"},
					})
					if err != nil {
						return err
					}

					if anyFailed {
						return &checksExitError{code: 1}
					}
					if anyRunning {
						return &checksExitError{code: 2}
					}
					return nil
				}

				time.Sleep(5 * time.Second)
			}
		},
	}

	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for all checks to reach a terminal state")
	cmd.Flags().IntVar(&timeout, "timeout", 600, "Maximum seconds to wait (with --wait)")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository (account/org/project/repo)")
	cmd.Flags().StringVar(&jsonFields, "json", "", "Output JSON with selected fields (e.g. check.identifier,check.status)")

	return cmd
}
