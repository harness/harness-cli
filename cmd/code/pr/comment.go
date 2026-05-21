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

func newCommentCmd(f *cmdutils.Factory) *cobra.Command {
	var (
		body       string
		dryRun     bool
		repo       string
		jsonFields string
	)

	cmd := &cobra.Command{
		Use:   "comment <number>",
		Short: "Add a comment to a pull request",
		Long:  "Post a comment on a pull request.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid PR number %q: %w", args[0], err)
			}

			if body == "" {
				return fmt.Errorf("--body is required")
			}

			repoRef, err := resolveRepoRef(repo)
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Fprintf(os.Stderr, `{"ts":"%s","op":"pr.comment","target":"%s#%d","dry_run":true}`+"\n",
					time.Now().UTC().Format(time.RFC3339), repoRef, number)
				fmt.Fprintf(os.Stdout, "Would post comment on PR #%d:\n%s\n", number, body)
				return nil
			}

			client := f.CodeClient()
			comment, err := client.CreateComment(repoRef, number, code.CreateCommentInput{
				Text: body,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, `{"ts":"%s","op":"pr.comment","target":"%s#%d","result":"success","dry_run":false}`+"\n",
				time.Now().UTC().Format(time.RFC3339), repoRef, number)

			if jsonFields != "" {
				return printJSON(comment, jsonFields)
			}

			fmt.Fprintf(os.Stdout, "Comment added (id: %d) on PR #%d\n", comment.ID, number)
			return nil
		},
	}

	cmd.Flags().StringVar(&body, "body", "", "Comment text (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be posted without executing")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository (account/org/project/repo)")
	cmd.Flags().StringVar(&jsonFields, "json", "", "Output JSON with selected fields (e.g. id,body,author)")

	return cmd
}
