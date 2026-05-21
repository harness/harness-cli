package pr

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/spf13/cobra"
)

func newViewCmd(f *cmdutils.Factory) *cobra.Command {
	var (
		repo       string
		jsonFields string
	)

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View a pull request",
		Long:  "Display details of a pull request including title, description, state, and checks summary.",
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
			pr, err := client.GetPullRequest(repoRef, number)
			if err != nil {
				return err
			}

			if jsonFields != "" {
				return printJSON(pr, jsonFields)
			}

			fmt.Fprintf(os.Stdout, "%s #%d\n", pr.Title, pr.Number)
			fmt.Fprintf(os.Stdout, "State:   %s\n", pr.State)
			fmt.Fprintf(os.Stdout, "Author:  %s\n", pr.Author.DisplayName)
			fmt.Fprintf(os.Stdout, "Branch:  %s -> %s\n", pr.SourceBranch, pr.TargetBranch)
			if pr.Description != "" {
				fmt.Fprintf(os.Stdout, "\n%s\n", pr.Description)
			}
			fmt.Fprintf(os.Stdout, "\nStats: +%d -%d across %d files, %d commits\n",
				pr.Stats.Additions, pr.Stats.Deletions, pr.Stats.FilesChanged, pr.Stats.Commits)
			if pr.Created > 0 {
				fmt.Fprintf(os.Stdout, "Created: %s\n", time.Unix(pr.Created/1000, 0).Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository (account/org/project/repo)")
	cmd.Flags().StringVar(&jsonFields, "json", "", "Output JSON with selected fields")

	return cmd
}
