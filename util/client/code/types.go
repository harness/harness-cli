package code

type PullRequest struct {
	Author struct {
		Created     int64  `json:"created"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		ID          int64  `json:"id"`
		Type        string `json:"type"`
		UID         string `json:"uid"`
		Updated     int64  `json:"updated"`
	} `json:"author"`
	Created          int64         `json:"created"`
	Description      string        `json:"description"`
	Edited           int64         `json:"edited"`
	IsDraft          bool          `json:"is_draft"`
	MergeBaseSha     string        `json:"merge_base_sha"`
	MergeCheckStatus string        `json:"merge_check_status"`
	MergeConflicts   []interface{} `json:"merge_conflicts"`
	MergeMethod      string        `json:"merge_method"`
	MergeSha         string        `json:"merge_sha"`
	MergeTargetSha   string        `json:"merge_target_sha"`
	Merged           int64         `json:"merged"`
	Merger           *User         `json:"merger"`
	Number           int64         `json:"number"`
	SourceBranch     string        `json:"source_branch"`
	SourceRepoID     int64         `json:"source_repo_id"`
	SourceSha        string        `json:"source_sha"`
	State            string        `json:"state"`
	Stats            PullRequestStats `json:"stats"`
	TargetBranch     string        `json:"target_branch"`
	TargetRepoID     int64         `json:"target_repo_id"`
	Title            string        `json:"title"`
	URL              string        `json:"pr_url"`
}

type PullRequestStats struct {
	Additions       int64 `json:"additions"`
	Commits         int64 `json:"commits"`
	Conversations   int64 `json:"conversations"`
	Deletions       int64 `json:"deletions"`
	FilesChanged    int64 `json:"files_changed"`
	UnresolvedCount int64 `json:"unresolved_count"`
}

type User struct {
	Created     int64  `json:"created"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	UID         string `json:"uid"`
	Updated     int64  `json:"updated"`
}

type ListPullRequestsOptions struct {
	States       []string
	SourceBranch string
	TargetBranch string
	Query        string
	CreatedBy    string
	Page         int
	Limit        int
	Order        string
	Sort         string
}

type CreatePullRequestInput struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	IsDraft      bool   `json:"is_draft"`
}

type MergePullRequestInput struct {
	Method    string `json:"method,omitempty"`
	SourceSHA string `json:"source_sha"`
	DryRun    bool   `json:"dry_run"`
}

type MergePullRequestResponse struct {
	SHA            string   `json:"sha,omitempty"`
	Mergeable      bool     `json:"mergeable"`
	ConflictFiles  []string `json:"conflict_files"`
	RuleViolations []struct {
		Violations []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"violations"`
	} `json:"rule_violations"`
}

type CreateCommentInput struct {
	Text     string `json:"text"`
	ParentID *int64 `json:"parent_id,omitempty"`
}

type Comment struct {
	ID      int64  `json:"id"`
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
	Text    string `json:"text"`
	Author  User   `json:"author"`
}

type ChecksResponse struct {
	CommitSHA string  `json:"commit_sha"`
	Checks    []Check `json:"checks"`
}

type Check struct {
	Required   bool        `json:"required"`
	Bypassable bool        `json:"bypassable"`
	Detail     CheckDetail `json:"check"`
}

type CheckDetail struct {
	ID         int64  `json:"id"`
	Created    int64  `json:"created"`
	Updated    int64  `json:"updated"`
	Identifier string `json:"identifier"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	Link       string `json:"link"`
	Started    int64  `json:"started"`
	Ended      int64  `json:"ended"`
	UID        string `json:"uid"`
}

type PullRequestReviewer struct {
	Created        int64       `json:"created"`
	Updated        int64       `json:"updated"`
	Type           string      `json:"type"`
	LatestReviewID interface{} `json:"latest_review_id"`
	ReviewDecision string      `json:"review_decision"`
	SHA            string      `json:"sha"`
	Reviewer       User        `json:"reviewer"`
	AddedBy        User        `json:"added_by"`
}
