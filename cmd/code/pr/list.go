package pr

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/harness/harness-cli/cmd/cmdutils"
	"github.com/harness/harness-cli/util/client/code"
	"github.com/harness/harness-cli/util/common/printer"
	"github.com/spf13/cobra"
)

func newListCmd(f *cmdutils.Factory) *cobra.Command {
	var (
		state      string
		author     string
		limit      int
		page       int
		repo       string
		jsonFields string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests",
		Long:  "List pull requests in a Harness Code repository.",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRef, err := resolveRepoRef(repo)
			if err != nil {
				return err
			}

			client := f.CodeClient()

			opts := code.ListPullRequestsOptions{
				Limit:    limit,
				Page:     page,
				CreatedBy: author,
				Sort:     "number",
				Order:    "desc",
			}
			if state != "" {
				opts.States = strings.Split(state, ",")
			}

			prs, err := client.ListPullRequests(repoRef, opts)
			if err != nil {
				return err
			}

			if jsonFields != "" {
				return printJSON(prs, jsonFields)
			}

			return printer.Print(prs, int64(page), 0, int64(len(prs)), false, [][]string{
				{"number", "Number"},
				{"title", "Title"},
				{"state", "State"},
				{"author.display_name", "Author"},
				{"source_branch", "Source"},
				{"target_branch", "Target"},
			})
		},
	}

	cmd.Flags().StringVar(&state, "state", "", "Filter by state (open, closed, merged)")
	cmd.Flags().StringVar(&author, "author", "", "Filter by author")
	cmd.Flags().IntVar(&limit, "limit", 30, "Maximum number of PRs to list")
	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository (account/org/project/repo). Auto-detected from git remote if omitted.")
	cmd.Flags().StringVar(&jsonFields, "json", "", "Output JSON with selected fields (e.g. number,title,state)")

	return cmd
}

func resolveRepoRef(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	ctx, err := cmdutils.ResolveRepo()
	if err != nil {
		return "", fmt.Errorf("could not detect repository: %w\nUse --repo to specify explicitly", err)
	}
	return ctx.RepoRef, nil
}

func printJSON(data interface{}, fields string) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if fields == "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	fieldList := strings.Split(fields, ",")
	for i := range fieldList {
		fieldList[i] = strings.TrimSpace(fieldList[i])
	}

	var items []interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		// Single object
		var obj map[string]interface{}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return json.NewEncoder(os.Stdout).Encode(data)
		}
		filtered := filterFields(obj, fieldList)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	var result []map[string]interface{}
	for _, item := range items {
		if obj, ok := item.(map[string]interface{}); ok {
			result = append(result, filterFields(obj, fieldList))
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func filterFields(obj map[string]interface{}, fields []string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, f := range fields {
		parts := strings.SplitN(f, ".", 2)
		if len(parts) == 2 {
			if nested, ok := obj[parts[0]].(map[string]interface{}); ok {
				if val, ok := nested[parts[1]]; ok {
					result[f] = val
				}
			}
		} else if val, ok := obj[f]; ok {
			result[f] = val
		}
	}
	return result
}
