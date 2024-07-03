package gitness

import "time"

type Organisation struct {
	ID   string
	Name string
}

type Project struct {
	ID          string
	Name        string
	Description string
}

type User struct {
	ID    string
	Email string
	Name  string
}

type PullRequest struct {
	ID     string
	Title  string
	Status string
	// Other relevant fields
}

type CreateProjectPayload struct {
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
	UID         string `json:"uid"`
	ParentID    int    `json:"parent_id"`
}

type CreateProjectResponse struct {
	ID          int    `json:"id"`
	ParentID    int    `json:"parent_id"`
	Path        string `json:"path"`
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
	CreatedBy   int    `json:"created_by"`
	Created     int64  `json:"created"`
	Updated     int64  `json:"updated"`
	UID         string `json:"uid"`
}

type CreateRepositoryPayload struct {
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
	GitIgnore     string `json:"git_ignore"`
	IsPublic      bool   `json:"is_public"`
	License       string `json:"license"`
	UID           string `json:"uid"`
	Readme        bool   `json:"readme"`
	ParentRef     string `json:"parent_ref"`
}

type CreateRepositoryResponse struct {
	ID             int    `json:"id"`
	ParentID       int    `json:"parent_id"`
	Identifier     string `json:"identifier"`
	Path           string `json:"path"`
	Description    string `json:"description"`
	IsPublic       bool   `json:"is_public"`
	CreatedBy      int    `json:"created_by"`
	Created        int64  `json:"created"`
	Updated        int64  `json:"updated"`
	Size           int    `json:"size"`
	SizeUpdated    int    `json:"size_updated"`
	DefaultBranch  string `json:"default_branch"`
	ForkID         int    `json:"fork_id"`
	NumForks       int    `json:"num_forks"`
	NumPulls       int    `json:"num_pulls"`
	NumClosedPulls int    `json:"num_closed_pulls"`
	NumOpenPulls   int    `json:"num_open_pulls"`
	NumMergedPulls int    `json:"num_merged_pulls"`
	Importing      bool   `json:"importing"`
	GitURL         string `json:"git_url"`
	UID            string `json:"uid"`
}

type CreateBranchPayload struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	BypassRules bool   `json:"bypass_rules"`
}

type CommitAuthor struct {
	Identity struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"identity"`
	When string `json:"when"`
}

type CommitStats struct {
	Total struct {
		Insertions int `json:"insertions"`
		Deletions  int `json:"deletions"`
		Changes    int `json:"changes"`
	} `json:"total"`
}

type Commit struct {
	SHA       string       `json:"sha"`
	Title     string       `json:"title"`
	Message   string       `json:"message"`
	Author    CommitAuthor `json:"author"`
	Committer CommitAuthor `json:"committer"`
	Stats     CommitStats  `json:"stats"`
}

type CreateBranchResponse struct {
	Name   string `json:"name"`
	SHA    string `json:"sha"`
	Commit Commit `json:"commit"`
}

type CreatePullRequestPayload struct {
	TargetBranch string `json:"target_branch"`
	SourceBranch string `json:"source_branch"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	IsDraft      bool   `json:"is_draft"`
}

type CreatePullRequestResponse struct {
	Number           int         `json:"number"`
	Created          int64       `json:"created"`
	Edited           int64       `json:"edited"`
	State            string      `json:"state"`
	IsDraft          bool        `json:"is_draft"`
	Title            string      `json:"title"`
	Description      string      `json:"description"`
	SourceRepoID     int         `json:"source_repo_id"`
	SourceBranch     string      `json:"source_branch"`
	SourceSHA        string      `json:"source_sha"`
	TargetRepoID     int         `json:"target_repo_id"`
	TargetBranch     string      `json:"target_branch"`
	Merged           interface{} `json:"merged"`
	MergeMethod      interface{} `json:"merge_method"`
	MergeCheckStatus string      `json:"merge_check_status"`
	MergeTargetSHA   interface{} `json:"merge_target_sha"`
	MergeBaseSHA     string      `json:"merge_base_sha"`
	Author           struct {
		ID          int    `json:"id"`
		UID         string `json:"uid"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Type        string `json:"type"`
		Created     int64  `json:"created"`
		Updated     int64  `json:"updated"`
	} `json:"author"`
	Merger interface{} `json:"merger"`
	Stats  struct{}    `json:"stats"`
}

type MergePullRequestPayload struct {
	Method      string `json:"method"`
	SourceSHA   string `json:"source_sha"`
	BypassRules bool   `json:"bypass_rules"`
	DryRun      bool   `json:"dry_run"`
}

type MergePullRequestResponse struct {
	SHA string `json:"sha"`
}

type FetchPullRequestResponse struct {
	Number           int    `json:"number"`
	Created          int64  `json:"created"`
	Edited           int64  `json:"edited"`
	State            string `json:"state"`
	IsDraft          bool   `json:"is_draft"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	SourceRepoID     int    `json:"source_repo_id"`
	SourceBranch     string `json:"source_branch"`
	SourceSHA        string `json:"source_sha"`
	TargetRepoID     int    `json:"target_repo_id"`
	TargetBranch     string `json:"target_branch"`
	Merged           int64  `json:"merged"`
	MergeMethod      string `json:"merge_method"`
	MergeCheckStatus string `json:"merge_check_status"`
	MergeTargetSHA   string `json:"merge_target_sha"`
	MergeBaseSHA     string `json:"merge_base_sha"`
	Author           struct {
		ID          int    `json:"id"`
		UID         string `json:"uid"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Type        string `json:"type"`
		Created     int64  `json:"created"`
		Updated     int64  `json:"updated"`
	} `json:"author"`
	Merger struct {
		ID          int    `json:"id"`
		UID         string `json:"uid"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Type        string `json:"type"`
		Created     int64  `json:"created"`
		Updated     int64  `json:"updated"`
	} `json:"merger"`
	Stats struct {
		Commits      int `json:"commits"`
		FilesChanged int `json:"files_changed"`
	} `json:"stats"`
}

type GetMainBranchCommitResponse struct {
	Commits []struct {
		SHA        string   `json:"sha"`
		ParentSHAs []string `json:"parent_shas"`
		Title      string   `json:"title"`
		Message    string   `json:"message"`
		Author     struct {
			Identity struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"identity"`
			When time.Time `json:"when"`
		} `json:"author"`
		Committer struct {
			Identity struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"identity"`
			When time.Time `json:"when"`
		} `json:"committer"`
		Stats struct {
			Total struct {
				Insertions int `json:"insertions"`
				Deletions  int `json:"deletions"`
				Changes    int `json:"changes"`
			} `json:"total"`
		} `json:"stats"`
	} `json:"commits"`
	RenameDetails []struct {
	} `json:"rename_details"`
	TotalCommits int `json:"total_commits"`
}
