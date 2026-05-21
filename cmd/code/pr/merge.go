package pr

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/util/client/code"
	"github.com/spf13/cobra"
)

func newMergeCmd(f *cmdutils.Factory) *cobra.Command {
	var (
		method     string
		yes        bool
		dryRun     bool
		repo       string
		jsonFields string
	)

	cmd := &cobra.Command{
		Use:   "merge <number>",
		Short: "Merge a pull request",
		Long:  "Merge a pull request. Use --yes to skip confirmation in non-interactive contexts.",
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

			// Fetch the PR to get source SHA and check state
			pr, err := client.GetPullRequest(repoRef, number)
			if err != nil {
				return err
			}

			// Idempotent: already merged is success
			if pr.State == "merged" {
				if jsonFields != "" {
					return printJSON(pr, jsonFields)
				}
				fmt.Fprintf(os.Stdout, "PR #%d is already merged.\n", number)
				return nil
			}

			if !yes && !dryRun {
				return fmt.Errorf("merge is a destructive operation; use --yes to confirm or --dry-run to preview")
			}

			input := code.MergePullRequestInput{
				Method:    method,
				SourceSHA: pr.SourceSha,
				DryRun:    dryRun,
			}

			if dryRun {
				fmt.Fprintf(os.Stderr, `{"ts":"%s","op":"pr.merge","target":"%s#%d","dry_run":true}`+"\n",
					time.Now().UTC().Format(time.RFC3339), repoRef, number)

				result, err := client.MergePullRequest(repoRef, number, input)
				if err != nil {
					return err
				}
				if !result.Mergeable {
					fmt.Fprintf(os.Stdout, "PR #%d is NOT mergeable. Conflicts: %v\n", number, result.ConflictFiles)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stdout, "PR #%d is mergeable (method: %s)\n", number, method)
				return nil
			}

			result, err := client.MergePullRequest(repoRef, number, input)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, `{"ts":"%s","op":"pr.merge","target":"%s#%d","result":"success","dry_run":false}`+"\n",
				time.Now().UTC().Format(time.RFC3339), repoRef, number)

			if jsonFields != "" {
				return printJSON(result, jsonFields)
			}

			if !result.Mergeable && len(result.ConflictFiles) > 0 {
				fmt.Fprintf(os.Stdout, "Merge failed: conflicts in %v\n", result.ConflictFiles)
				os.Exit(1)
			}

			fmt.Fprintf(os.Stdout, "Merged PR #%d (method: %s)\n", number, method)
			return nil
		},
	}

	cmd.Flags().StringVar(&method, "method", "squash", "Merge method: squash, merge, or rebase")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Check mergeability without merging")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository (account/org/project/repo)")
	cmd.Flags().StringVar(&jsonFields, "json", "", "Output JSON with selected fields")

	return cmd
}
