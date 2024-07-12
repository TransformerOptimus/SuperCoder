package gitness_git_provider

import (
	"ai-developer/app/client"
	"ai-developer/app/models/dtos/gitness"
	"ai-developer/app/monitoring"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
)

type GitnessClient struct {
	baseURL    string
	authToken  string
	httpClient *client.HttpClient
	slackAlert *monitoring.SlackAlert
	logger     *zap.Logger
}

func NewGitnessClient(
	baseURL, authToken string,
	httpClient *client.HttpClient,
	logger *zap.Logger,
	slackAlert *monitoring.SlackAlert,
) *GitnessClient {
	return &GitnessClient{
		baseURL:    baseURL,
		authToken:  authToken,
		httpClient: httpClient,
		logger:     logger.Named("GitnessClient"),
		slackAlert: slackAlert,
	}
}

func (c *GitnessClient) CreateProject(name, description string) (*gitness.Project, error) {
	url := fmt.Sprintf("%s/api/v1/spaces", c.baseURL)

	payload := gitness.CreateProjectPayload{
		Description: description,
		IsPublic:    false,
		UID:         name,
		ParentID:    0,
	}

	headers := map[string]string{
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Post(url, payload, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create project, status code: %d", response.StatusCode)
	}

	var createProjectResponse gitness.CreateProjectResponse
	if err := json.NewDecoder(response.Body).Decode(&createProjectResponse); err != nil {
		return nil, err
	}

	return &gitness.Project{
		ID:          fmt.Sprintf("%d", createProjectResponse.ID),
		Name:        createProjectResponse.Identifier,
		Description: createProjectResponse.Description,
	}, nil
}

func (c *GitnessClient) CreateRepository(parentRef, uid, description, defaultBranch, license string, isPublic, readme bool) (*gitness.CreateRepositoryResponse, error) {
	url := fmt.Sprintf("%s/api/v1/repos", c.baseURL)

	payload := gitness.CreateRepositoryPayload{
		DefaultBranch: defaultBranch,
		Description:   description,
		IsPublic:      isPublic,
		License:       license,
		UID:           uid,
		Readme:        readme,
		ParentRef:     parentRef,
	}
	headers := map[string]string{
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Post(url, payload, headers)
	if err != nil {
		c.logger.Error(
			"Error creating repository",
			zap.Error(err),
			zap.String("url", url),
			zap.Any("payload", payload),
		)
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		c.logger.Error(
			"Error creating repository",
			zap.Error(err),
			zap.String("url", url),
			zap.Any("payload", payload),
		)
		return nil, fmt.Errorf("failed to create repository, status code: %d", response.StatusCode)
	}

	var createRepoResponse gitness.CreateRepositoryResponse
	if err := json.NewDecoder(response.Body).Decode(&createRepoResponse); err != nil {
		return nil, err
	}

	return &createRepoResponse, nil
}

func (c *GitnessClient) CreateBranch(repoPath, branchName, target string, bypassRules bool) (*gitness.CreateBranchResponse, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/branches", c.baseURL, repoPath)

	payload := gitness.CreateBranchPayload{
		Name:        branchName,
		Target:      target,
		BypassRules: bypassRules,
	}

	headers := map[string]string{
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Post(url, payload, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create branch, status code: %d", response.StatusCode)
	}

	var createBranchResponse gitness.CreateBranchResponse
	if err := json.NewDecoder(response.Body).Decode(&createBranchResponse); err != nil {
		return nil, err
	}

	return &createBranchResponse, nil
}

func (c *GitnessClient) CreatePullRequest(repoPath, sourceBranch, targetBranch, title, description string, isDraft bool) (*gitness.CreatePullRequestResponse, error) {
	fmt.Println("Making Pull Requst API call")
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/pullreq", c.baseURL, repoPath)

	payload := gitness.CreatePullRequestPayload{
		TargetBranch: targetBranch,
		SourceBranch: sourceBranch,
		Title:        title,
		Description:  description,
		IsDraft:      isDraft,
	}
	//fmt.Println("__token___", c.authToken)
	headers := map[string]string{
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Post(url, payload, headers)
	if err != nil {
		fmt.Printf("Error creating pull request: %v\n", err)
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create pull request, status code: %d", response.StatusCode)
	}

	var createPullRequestResponse gitness.CreatePullRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&createPullRequestResponse); err != nil {
		return nil, err
	}
	fmt.Printf("Pull Request Response: %v\n", createPullRequestResponse)
	return &createPullRequestResponse, nil
}

func (c *GitnessClient) MergePullRequest(repoPath string, pullRequestID int, method, sourceSHA string, bypassRules, dryRun bool) (*gitness.MergePullRequestResponse, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/pullreq/%d/merge", c.baseURL, repoPath, pullRequestID)

	payload := gitness.MergePullRequestPayload{
		Method:      method,
		SourceSHA:   sourceSHA,
		BypassRules: bypassRules,
		DryRun:      dryRun,
	}

	headers := map[string]string{
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Post(url, payload, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	fmt.Println("Response-->", response.StatusCode)
	fmt.Println("Response Body: ", response.Body)
	if response.StatusCode != http.StatusOK {
		err := c.slackAlert.SendAlert(fmt.Sprintf("Failed to merge pull request, status code: %d", response.StatusCode), map[string]string{
			"repoPath":      repoPath,
			"pullRequestID": fmt.Sprintf("%d", pullRequestID),
		})
		if err != nil {
			fmt.Println("Error sending slack alert: ", err)
			return nil, err
		}
		return nil, fmt.Errorf("failed to merge pull request, status code: %d", response.StatusCode)
	}

	var mergePullRequestResponse gitness.MergePullRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&mergePullRequestResponse); err != nil {
		return nil, err
	}
	fmt.Println("merge pull request Response:", mergePullRequestResponse)
	return &mergePullRequestResponse, nil
}

func (c *GitnessClient) FetchPullRequest(repoPath string, pullRequestID int) (*gitness.FetchPullRequestResponse, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/pullreq/%d", c.baseURL, repoPath, pullRequestID)

	headers := map[string]string{
		"Accept":        "*/*",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Get(url, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch pull request, status code: %d", response.StatusCode)
	}

	var fetchPullRequestResponse gitness.FetchPullRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&fetchPullRequestResponse); err != nil {
		return nil, err
	}

	return &fetchPullRequestResponse, nil
}

func (c *GitnessClient) GetPullRequestDiff(repoPath, fromSHA, toSHA string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/diff/%s...%s", c.baseURL, repoPath, fromSHA, toSHA)
	fmt.Println("RepoPath: ", repoPath)
	fmt.Println("FromSHA: ", fromSHA)
	fmt.Println("ToSHA: ", toSHA)
	fmt.Println("URL: ", url)
	fmt.Println("AuthToken: ", c.authToken)
	headers := map[string]string{
		"Accept":        "text/plain",
		"Authorization": "Bearer " + c.authToken,
	}
	fmt.Printf("HTTP Client: %v\n", c.httpClient)
	response, err := c.httpClient.Get(url, headers)
	if err != nil {
		fmt.Printf("Error getting pull request diff: %v\n", err)
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get pull request diff, status code: %d", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (c *GitnessClient) GetPullRequestCommits(repoPath string, pullRequestNumber int) (string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/pullreq/%d/commits?page=1&limit=100", c.baseURL, repoPath, pullRequestNumber)
	fmt.Println("URL: ", url)
	fmt.Println("BaseURL: ", c.baseURL)
	fmt.Println("RepoPath: ", repoPath)
	fmt.Println("PullRequestNumber: ", pullRequestNumber)
	fmt.Println("____AuthToken: ___", c.authToken)

	headers := map[string]string{
		"Accept":        "*/*",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Get(url, headers)
	if err != nil {
		fmt.Printf("Error getting pull request commits: %v\n", err)
		return "", err
	}
	defer response.Body.Close()
	fmt.Println("Response Status: ", response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch pull request, status code: %d", response.StatusCode)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (c *GitnessClient) GetBranchCommits(repoPath string, branchName ...string) (*gitness.GetMainBranchCommitResponse, error) {
	var branch string
	if len(branchName) > 0 {
		branch = branchName[0]
	} else {
		branch = "main"
	}
	url := fmt.Sprintf("%s/api/v1/repos/%s/+/commits?git_ref=%s&include_stats=false", c.baseURL, repoPath, branch)
	fmt.Println("URL: ", url)
	fmt.Println("BaseURL: ", c.baseURL)
	fmt.Println("RepoPath: ", repoPath)

	headers := map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + c.authToken,
	}

	response, err := c.httpClient.Get(url, headers)
	if err != nil {
		fmt.Printf("Error getting pull request commits: %v\n", err)
		return nil, err
	}
	defer response.Body.Close()
	fmt.Println("Response Status: ", response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch pull request, status code: %d", response.StatusCode)
	}

	var getMainBranchCommitResponse gitness.GetMainBranchCommitResponse
	if err := json.NewDecoder(response.Body).Decode(&getMainBranchCommitResponse); err != nil {
		return nil, err
	}

	return &getMainBranchCommitResponse, nil
}
