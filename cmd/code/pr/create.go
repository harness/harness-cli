package pr

import (
	"fmt"
	"os"
	"time"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/util/client/code"
	"github.com/spf13/cobra"
)

func newCreateCmd(f *cmdutils.Factory) *cobra.Command {
	var (
		title      string
		body       string
		head       string
		base       string
		draft      bool
		dryRun     bool
		repo       string
		jsonFields string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long:  "Create a new pull request in a Harness Code repository.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if head == "" {
				return fmt.Errorf("--head is required")
			}

			repoRef, err := resolveRepoRef(repo)
			if err != nil {
				return err
			}

			input := code.CreatePullRequestInput{
				Title:        title,
				Description:  body,
				SourceBranch: head,
				TargetBranch: base,
				IsDraft:      draft,
			}

			if dryRun {
				fmt.Fprintf(os.Stderr, `{"ts":"%s","op":"pr.create","target":"%s","dry_run":true}`+"\n",
					time.Now().UTC().Format(time.RFC3339), repoRef)
				fmt.Fprintf(os.Stdout, "Would create PR: %q (%s -> %s)\n", title, head, base)
				return nil
			}

			client := f.CodeClient()
			pr, err := client.CreatePullRequest(repoRef, input)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, `{"ts":"%s","op":"pr.create","target":"%s#%d","result":"success","dry_run":false}`+"\n",
				time.Now().UTC().Format(time.RFC3339), repoRef, pr.Number)

			if jsonFields != "" {
				return printJSON(pr, jsonFields)
			}

			fmt.Fprintf(os.Stdout, "Created PR #%d: %s\n", pr.Number, pr.Title)
			if pr.URL != "" {
				fmt.Fprintf(os.Stdout, "%s\n", pr.URL)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Title for the pull request (required)")
	cmd.Flags().StringVar(&body, "body", "", "Description body for the pull request")
	cmd.Flags().StringVar(&head, "head", "", "Source branch name (required)")
	cmd.Flags().StringVar(&base, "base", "", "Target branch name (defaults to repo default branch)")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create as a draft pull request")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be created without executing")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository (account/org/project/repo)")
	cmd.Flags().StringVar(&jsonFields, "json", "", "Output JSON with selected fields (e.g. number,url)")

	return cmd
}

