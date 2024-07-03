package git_providers

import (
	"ai-developer/app/models/dtos/gitness"
)

type GitService interface {
	CreateProject(name, description string) (*gitness.Project, error)
	CreateRepository(projectName, repoName, description string) (*gitness.CreateRepositoryResponse, error)
	CreateBranch(projectName, repoName, branchName string) (*gitness.CreateBranchResponse, error)
	CreatePullRequest(projectName, repoName, sourceBranch, targetBranch, title, description string) (*gitness.CreatePullRequestResponse, error)
	MergePullRequest(projectName, repoName string, pullRequestID int, sourceSHA string) (*gitness.MergePullRequestResponse, error)
	FetchPullRequest(projectName, repoName string, pullRequestID int) (*gitness.FetchPullRequestResponse, error)
}
