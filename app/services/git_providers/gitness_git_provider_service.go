package git_providers

import (
	"ai-developer/app/client/git_provider"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/gitness"
	"fmt"
	"strconv"
)

type GitnessService struct {
	client *gitness_git_provider.GitnessClient
}

func NewGitnessService(client *gitness_git_provider.GitnessClient) *GitnessService {
	return &GitnessService{client: client}
}

func (s *GitnessService) CreateProject(name, description string) (*gitness.Project, error) {
	fmt.Printf("Creating Gitness project %s\n", name)
	project, err := s.client.CreateProject(name, description)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *GitnessService) CreateRepository(projectName, repoName, description string) (*gitness.CreateRepositoryResponse, error) {
	defaultBranch := "main"
	license := "none"
	isPublic := true
	readme := false

	repo, err := s.client.CreateRepository(projectName, repoName, description, defaultBranch, license, isPublic, readme)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (s *GitnessService) CreateBranch(projectName, repoName, branchName string) (*gitness.CreateBranchResponse, error) {
	repoPath := fmt.Sprintf("%s/%s", projectName, repoName)
	target := "refs/heads/main"
	bypassRules := false

	branch, err := s.client.CreateBranch(repoPath, branchName, target, bypassRules)
	if err != nil {
		return nil, err
	}
	return branch, nil
}

func (s *GitnessService) CreatePullRequest(projectName, repoName, sourceBranch, targetBranch, title, description string) (*gitness.CreatePullRequestResponse, error) {
	fmt.Printf("Creating pull request for %s/%s\n", projectName, repoName)
	repoPath := fmt.Sprintf("%s/%s", projectName, repoName)
	isDraft := false

	pr, err := s.client.CreatePullRequest(repoPath, sourceBranch, targetBranch, title, description, isDraft)
	fmt.Println("Created pull request: ", pr)
	if err != nil {
		fmt.Printf("Error creating pull request from gitservice: %v\n", err)
		return nil, err
	}
	return pr, nil
}

func (s *GitnessService) MergePullRequest(projectName, repoName string, pullRequestID int, sourceSHA string) (*gitness.MergePullRequestResponse, error) {
	fmt.Println("Project Name: ", projectName)
	fmt.Println("Repo Name: ", repoName)
	fmt.Println("Pull Request ID: ", pullRequestID)
	fmt.Println("Source SHA: ", sourceSHA)

	repoPath := fmt.Sprintf("%s/%s", projectName, repoName)
	method := "squash"
	bypassRules := false
	dryRun := false

	merge, err := s.client.MergePullRequest(repoPath, pullRequestID, method, sourceSHA, bypassRules, dryRun)
	fmt.Println("Merged pull request: ", merge)
	if err != nil {
		return nil, err
	}
	return merge, nil
}

func (s *GitnessService) FetchPullRequest(projectName, repoName string, pullRequestID int) (*gitness.FetchPullRequestResponse, error) {
	repoPath := fmt.Sprintf("%s/%s", projectName, repoName)

	pr, err := s.client.FetchPullRequest(repoPath, pullRequestID)
	if err != nil {
		return nil, err
	}
	return pr, nil
}

func (s *GitnessService) FetchPullRequestCommits(projectName, repoName string, pullRequestNumber int) (string, error) {
	repoPath := fmt.Sprintf("%s/%s", projectName, repoName)
	commitsResponse, err := s.client.GetPullRequestCommits(repoPath, pullRequestNumber)
	if err != nil {
		return "", err
	}
	return commitsResponse, nil
}

func (s *GitnessService) GetPullRequestDiff(projectName, repoName, fromSHA, toSHA string) (string, error) {
	repoPath := fmt.Sprintf("%s/%s", projectName, repoName)

	diff, err := s.client.GetPullRequestDiff(repoPath, fromSHA, toSHA)
	if err != nil {
		return "", err
	}

	return diff, nil
}

func (s *GitnessService) GetSpaceOrProjectName(organisation *models.Organisation) string {
	return organisation.Name + "_" + strconv.Itoa(int(organisation.ID))

}

func (s *GitnessService) GetSpaceOrProjectDescription(organisation *models.Organisation) string {
	return "Space for " + organisation.Name + " organisation"

}

func (s *GitnessService) GetAllCommitsOfProjectBranch(organisation *models.Organisation, projectName string) (*gitness.GetMainBranchCommitResponse, error) {
	spaceOrProjectName := s.GetSpaceOrProjectName(organisation)
	repoPath := fmt.Sprintf("%s/%s", spaceOrProjectName, projectName)

	commitResponse, err := s.client.GetBranchCommits(repoPath)

	if err != nil {
		return nil, err
	}
	return commitResponse, nil
}
