package vcs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/harness/harness-cli/util/common/errors"
	"github.com/harness/harness-cli/util/common/fileutil"

	"gopkg.in/ini.v1"
)

// GitInfo represents Git repository metadata
type GitInfo struct {
	VCS  string `json:"VCS,omitempty"`
	URL  string `json:"URL,omitempty"`
	Ref  string `json:"Ref,omitempty"`
	Hash string `json:"Hash,omitempty"`
}

// validateGitPath checks if a path is valid and points to a Git repository.
// Returns an error if the path is empty, contains invalid characters,
// or if the path is not accessible.
func validateGitPath(path string) error {
	if path == "" {
		return errors.NewValidationError("path", "path cannot be empty")
	}

	// Check for invalid characters in path
	if strings.ContainsAny(path, "<>:|?*\\") {
		return errors.NewValidationError("path", "path contains invalid characters")
	}

	// Check if path exists and is accessible
	info, err := os.Stat(path)
	if err != nil {
		return errors.NewFileError(path, "access", err)
	}

	// Check if path is a directory
	if !info.IsDir() {
		return errors.NewValidationError("path", "path is not a directory")
	}

	// Check if path contains a .git directory
	gitDir := filepath.Join(path, ".git")
	gitInfo, err := os.Stat(gitDir)
	if err != nil {
		return errors.NewVCSError("validate", path, errors.ErrInvalidOperation)
	}
	if !gitInfo.IsDir() {
		return errors.NewVCSError("validate", path, errors.ErrInvalidOperation)
	}

	return nil
}

// GitRepository represents a Git repository
type GitRepository struct {
	path string
}

// NewGitRepository creates a new GitRepository instance.
// It validates that the provided path points to a valid Git repository.
// Returns a GitRepository instance or nil if validation fails.
func NewGitRepository(path string) *GitRepository {
	if err := validateGitPath(path); err != nil {
		return nil
	}
	return &GitRepository{
		path: path,
	}
}

// GetInfo returns Git repository information.
// It validates the repository state and extracts information about
// the current HEAD, remote URL, and commit hash.
//
// The returned GitInfo includes:
//   - VCS: the version control system type (always "git")
//   - URL: the remote repository URL
//   - Ref: the current branch or reference
//   - Hash: the full commit hash
//
// Returns an error if:
//   - The repository is invalid or inaccessible
//   - Required Git files cannot be read
//   - Repository state is inconsistent
func (g *GitRepository) GetInfo() (*GitInfo, error) {
	// Validate repository path
	if g == nil || g.path == "" {
		return nil, errors.NewVCSError("validate", "<nil>", errors.ErrInvalidOperation)
	}

	// Check Git directory
	gitDir := filepath.Join(g.path, ".git")
	if !fileutil.IsDir(gitDir) {
		return nil, errors.NewVCSError("validate", g.path, errors.ErrInvalidOperation)
	}

	info := &GitInfo{
		VCS: "git",
	}

	// Read and validate HEAD
	headPath := filepath.Join(gitDir, "HEAD")
	headBytes, err := fileutil.ReadFile(headPath)
	if err != nil {
		return nil, errors.NewVCSError("read_head", g.path, err)
	}
	head := strings.TrimSpace(string(headBytes))
	if head == "" {
		return nil, errors.NewVCSError("validate_head", g.path, errors.ErrInvalidOperation)
	}

	// Extract ref and hash
	if strings.HasPrefix(head, "ref: ") {
		ref := strings.TrimPrefix(head, "ref: ")
		info.Ref = ref

		// Validate and read ref file
		refPath := filepath.Join(gitDir, ref)
		if _, err := os.Stat(refPath); err != nil {
			return nil, errors.NewVCSError("validate_ref", g.path, err)
		}

		shaBytes, err := fileutil.ReadFile(refPath)
		if err != nil {
			return nil, errors.NewVCSError("read_ref", g.path, err)
		}

		hash := strings.TrimSpace(string(shaBytes))
		if !isValidSHA(hash) {
			return nil, errors.NewVCSError("validate_hash", g.path, errors.ErrInvalidOperation)
		}
		info.Hash = hash
	} else {
		// Validate detached HEAD hash
		if !isValidSHA(head) {
			return nil, errors.NewVCSError("validate_hash", g.path, errors.ErrInvalidOperation)
		}
		info.Hash = head
	}

	// Get and validate remote URL
	url, err := g.GetRemoteURL()
	if err != nil {
		return nil, err
	}
	info.URL = url

	return info, nil
}

// IsGitRepository checks if the given path is a Git repository
func IsGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	return fileutil.IsDir(gitDir)
}

// isValidSHA checks if a string is a valid Git SHA-1 hash.
// A valid SHA-1 hash is 40 characters long and contains only hexadecimal digits.
func isValidSHA(hash string) bool {
	if len(hash) != 40 {
		return false
	}
	for _, c := range hash {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// GetCurrentBranch returns the current branch name.
// If HEAD points to a branch, returns the branch name.
// If HEAD is detached, returns an empty string.
//
// Returns an error if:
//   - The repository is invalid or inaccessible
//   - HEAD file cannot be read
//   - HEAD content is invalid
func (g *GitRepository) GetCurrentBranch() (string, error) {
	// Validate repository
	if g == nil || g.path == "" {
		return "", errors.NewVCSError("validate", "<nil>", errors.ErrInvalidOperation)
	}

	// Read HEAD file
	gitDir := filepath.Join(g.path, ".git")
	headPath := filepath.Join(gitDir, "HEAD")

	headBytes, err := fileutil.ReadFile(headPath)
	if err != nil {
		return "", errors.NewVCSError("read_head", g.path, err)
	}

	// Parse and validate HEAD content
	head := strings.TrimSpace(string(headBytes))
	if head == "" {
		return "", errors.NewVCSError("validate_head", g.path, errors.ErrInvalidOperation)
	}

	// Extract branch name if HEAD points to a branch
	if strings.HasPrefix(head, "ref: refs/heads/") {
		branch := strings.TrimPrefix(head, "ref: refs/heads/")
		if branch == "" {
			return "", errors.NewVCSError("validate_branch", g.path, errors.ErrInvalidOperation)
		}
		return branch, nil
	}

	// HEAD is detached (points directly to a commit)
	return "", nil
}

// GetRemoteURL returns the remote URL of the repository.
// It reads the repository's config file and extracts the URL
// of the 'origin' remote.
//
// Returns an error if:
//   - The repository is invalid or inaccessible
//   - Config file cannot be read or parsed
//   - Remote 'origin' is not configured
func (g *GitRepository) GetRemoteURL() (string, error) {
	// Validate repository
	if g == nil || g.path == "" {
		return "", errors.NewVCSError("validate", "<nil>", errors.ErrInvalidOperation)
	}

	// Check config file
	gitDir := filepath.Join(g.path, ".git")
	configPath := filepath.Join(gitDir, "config")

	if !fileutil.IsFile(configPath) {
		return "", errors.NewVCSError("read_config", g.path, errors.ErrNotFound)
	}

	// Read and parse config file
	cfg, err := ini.Load(configPath)
	if err != nil {
		return "", errors.NewVCSError("read_config", g.path, err)
	}

	// Get and validate remote URL
	section := cfg.Section(`remote "origin"`)
	if section == nil {
		return "", errors.NewVCSError("validate_remote", g.path, errors.ErrNotFound)
	}

	url := section.Key("url").String()
	return url, nil
}

// GetCommitHash returns the current commit hash
func (g *GitRepository) GetCommitHash() (string, error) {
	gitDir := filepath.Join(g.path, ".git")
	headPath := filepath.Join(gitDir, "HEAD")

	headBytes, err := fileutil.ReadFile(headPath)
	if err != nil {
		return "", errors.NewVCSError("read_head", g.path, err)
	}

	head := strings.TrimSpace(string(headBytes))
	if strings.HasPrefix(head, "ref: ") {
		ref := strings.TrimPrefix(head, "ref: ")
		refPath := filepath.Join(gitDir, ref)
		shaBytes, err := fileutil.ReadFile(refPath)
		if err != nil {
			return "", errors.NewVCSError("read_ref", g.path, err)
		}
		return strings.TrimSpace(string(shaBytes)), nil
	}

	return head, nil // Detached HEAD, HEAD is already a commit hash
}
