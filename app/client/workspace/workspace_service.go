package workspace

import (
	"ai-developer/app/config"
	"ai-developer/app/monitoring"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
)

type WorkspaceServiceClient struct {
	endpoint   string
	client     *http.Client
	slackAlert *monitoring.SlackAlert
}

func (ws *WorkspaceServiceClient) CreateWorkspace(createWorkspaceRequest *request.CreateWorkspaceRequest) (createWorkspaceResponse *response.CreateWorkspaceResponse, err error) {
	payload, err := json.Marshal(createWorkspaceRequest)
	if err != nil {
		log.Printf("failed to marshal create workspace request: %v", err)
		return
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/workspaces", ws.endpoint), bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("failed to create create workspace request: %v", err)
		return
	}
	res, err := ws.client.Do(req)
	if err != nil {
		log.Printf("failed to send create workspace request: %v", err)
		return
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		err := ws.slackAlert.SendAlert(fmt.Sprintf("failed to create workspace: %s", res.Status), map[string]string{
			"workspace_id": createWorkspaceRequest.WorkspaceId,
		})
		if err != nil {
			log.Printf("failed to send slack alert: %v", err)
			return nil, err
		}
		return nil, errors.New(fmt.Sprintf("invalid res from workspace service for create workspace request"))
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("failed to read res payload: %v", err)
		return
	}

	createWorkspaceResponse = &response.CreateWorkspaceResponse{}
	if err1 := json.Unmarshal(responseBody, &createWorkspaceResponse); err1 != nil {
		log.Printf("failed to unmarshal create workspace res: %v", err1)
		return
	}
	return
}

func (ws *WorkspaceServiceClient) CreateFrontendWorkspace(createWorkspaceRequest *request.CreateWorkspaceRequest) (createWorkspaceResponse *response.CreateWorkspaceResponse, err error) {
	payload, err := json.Marshal(createWorkspaceRequest)
	if err != nil {
		log.Printf("failed to marshal create workspace request: %v", err)
		return
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/frontend/workspaces", ws.endpoint), bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("failed to create create workspace request: %v", err)
		return
	}
	res, err := ws.client.Do(req)
	if err != nil {
		log.Printf("failed to send create workspace request: %v", err)
		return
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.New(fmt.Sprintf("invalid res from workspace service for create workspace request"))
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("failed to read res payload: %v", err)
		return
	}

	createWorkspaceResponse = &response.CreateWorkspaceResponse{}
	if err1 := json.Unmarshal(responseBody, &createWorkspaceResponse); err1 != nil {
		log.Printf("failed to unmarshal create workspace res: %v", err1)
		return
	}
	return
}

func (ws *WorkspaceServiceClient) DeleteWorkspace(workspaceId string) (err error) {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/workspaces/%s", ws.endpoint, workspaceId), nil)
	if err != nil {
		return
	}
	res, err := ws.client.Do(req)
	if err != nil {
		return
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		err = ws.slackAlert.SendAlert(fmt.Sprintf("failed to delete workspace: %s", res.Status), map[string]string{
			"workspace_id": workspaceId,
		})
		if err != nil {
			log.Printf("failed to send slack alert: %v", err)
			return
		}

		return errors.New(fmt.Sprintf("invalid res from workspace service for delete workspace request %s", workspaceId))
	}
	return
}

func (ws *WorkspaceServiceClient) CreateJob(createJobRequest *request.CreateJobRequest) (createJobResponse *request.CreateJobResponse, err error) {
	payload, err := json.Marshal(createJobRequest)
	if err != nil {
		log.Printf("failed to marshal create job request: %v", err)
		return
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/jobs", ws.endpoint), bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("failed to create create job request: %v", err)
		return
	}
	res, err := ws.client.Do(req)
	if err != nil {
		log.Printf("failed to send create job request: %v", err)
		return
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		err := ws.slackAlert.SendAlert(fmt.Sprintf("failed to create execution job: %s", res.Status), map[string]string{
			"execution_id": strconv.FormatInt(createJobRequest.ExecutionId, 10),
			"story_id":     strconv.FormatInt(createJobRequest.StoryId, 10),
			"project_id":   createJobRequest.ProjectId,
		})
		if err != nil {
			log.Printf("failed to send slack alert: %v", err)
			return nil, err
		}
		return nil, errors.New(fmt.Sprintf("invalid response from workspace service for create job request"))
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("failed to read res payload: %v", err)
		return
	}

	createJobResponse = &request.CreateJobResponse{}
	if err1 := json.Unmarshal(responseBody, &createJobResponse); err1 != nil {
		log.Printf("failed to unmarshal create job res: %v", err1)
		return
	}
	return
}

func NewWorkspaceServiceClient(
	config *config.WorkspaceServiceConfig,
	client *http.Client,
	slackAlert *monitoring.SlackAlert,
) *WorkspaceServiceClient {
	return &WorkspaceServiceClient{
		endpoint:   config.GetEndpoint(),
		client:     client,
		slackAlert: slackAlert,
	}
}
