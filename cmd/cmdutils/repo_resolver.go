package cmdutils

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type RepoContext struct {
	RepoRef   string // e.g. "l7B_kbSEQD2wjrM7PShm5w/default/CD/codepulse"
	BaseURL   string // e.g. "https://harness0.harness.io/gateway"
	AccountID string // e.g. "l7B_kbSEQD2wjrM7PShm5w"
}

// ResolveRepo detects Harness Code repo context from the git remote origin URL.
// Supports URLs like:
//   - https://git0.harness.io/{accountId}/{repo}.git
//   - https://git0.harness.io/{accountId}/{org}/{repo}.git
//   - https://git0.harness.io/{accountId}/{org}/{project}/{repo}.git
//   - git@git0.harness.io:{accountId}/{org}/{project}/{repo}.git
func ResolveRepo() (*RepoContext, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git remote URL (are you in a git repository?): %w", err)
	}

	remoteURL := strings.TrimSpace(string(out))
	if remoteURL == "" {
		return nil, fmt.Errorf("git remote origin URL is empty")
	}

	return ParseRemoteURL(remoteURL)
}

// ParseRemoteURL parses a Harness Code git remote URL and extracts repo context.
func ParseRemoteURL(remoteURL string) (*RepoContext, error) {
	// Handle SSH URLs: git@git0.harness.io:account/org/project/repo.git
	if strings.HasPrefix(remoteURL, "git@") {
		return parseSSHRemote(remoteURL)
	}

	parsed, err := url.Parse(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote URL: %w", err)
	}

	if !strings.Contains(parsed.Host, "harness.io") {
		return nil, fmt.Errorf("remote URL is not a Harness Code repository: %s", remoteURL)
	}

	repoPath := strings.TrimPrefix(parsed.Path, "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")

	if repoPath == "" {
		return nil, fmt.Errorf("could not extract repo path from URL: %s", remoteURL)
	}

	parts := strings.SplitN(repoPath, "/", 2)
	accountID := parts[0]

	apiHost := parsed.Host
	if strings.HasPrefix(apiHost, "git") {
		apiHost = "harness" + strings.TrimPrefix(apiHost, "git")
	}
	baseURL := fmt.Sprintf("%s://%s/gateway", parsed.Scheme, apiHost)

	return &RepoContext{
		RepoRef:   repoPath,
		BaseURL:   baseURL,
		AccountID: accountID,
	}, nil
}

func parseSSHRemote(remoteURL string) (*RepoContext, error) {
	// git@git0.harness.io:account/org/project/repo.git
	atIdx := strings.Index(remoteURL, "@")
	colonIdx := strings.Index(remoteURL, ":")
	if atIdx < 0 || colonIdx < 0 || colonIdx <= atIdx {
		return nil, fmt.Errorf("invalid SSH remote URL format: %s", remoteURL)
	}

	host := remoteURL[atIdx+1 : colonIdx]
	if !strings.Contains(host, "harness.io") {
		return nil, fmt.Errorf("remote URL is not a Harness Code repository: %s", remoteURL)
	}

	repoPath := remoteURL[colonIdx+1:]
	repoPath = strings.TrimSuffix(repoPath, ".git")

	if repoPath == "" {
		return nil, fmt.Errorf("could not extract repo path from URL: %s", remoteURL)
	}

	parts := strings.SplitN(repoPath, "/", 2)
	accountID := parts[0]

	apiHost := host
	if strings.HasPrefix(apiHost, "git") {
		apiHost = "harness" + strings.TrimPrefix(apiHost, "git")
	}
	baseURL := fmt.Sprintf("https://%s/gateway", apiHost)

	return &RepoContext{
		RepoRef:   repoPath,
		BaseURL:   baseURL,
		AccountID: accountID,
	}, nil
}
