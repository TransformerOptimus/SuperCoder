import api from './apiConfig';
import {
  CreateProjectPayload,
  UpdateProjectPayload,
} from '../../types/projectsTypes';
import { FormStoryPayload } from '../../types/storyTypes';
import { CommentReBuildPayload } from '../../types/pullRequestsTypes';
import { CreateOrUpdateLLMAPIKeyPayload } from '../../types/modelsTypes';
import { authPayload } from '../../types/authTypes';

export const checkHealth = () => {
  return api.get(`/health`);
};

// Auth APIS
export const checkUserEmailExists = (user_email: string) => {
  return api.get(`/user/check_user`, {
    params: { user_email },
  });
};

export const login = (payload: authPayload) => {
  return api.post(`/user/sign_in`, payload);
};

export const signUp = (payload: authPayload) => {
  return api.post(`/user/sign_up`, payload);
};

// Project APIs
export const getAllProject = () => {
  return api.get(`/projects`);
};

export const getProjectById = (project_id: string) => {
  return api.get(`/projects/${project_id}`);
};

export const createProject = (payload: CreateProjectPayload) => {
  return api.post(`/projects`, payload);
};

export const updateProject = (payload: UpdateProjectPayload) => {
  return api.put(`/projects`, payload);
};

// Story APIs
export const createStory = (payload: FormStoryPayload) => {
  return api.post(`/stories`, payload);
};

export const editStory = (payload: FormStoryPayload) => {
  return api.post(`/stories/${payload.story_id}`, payload);
};

export const getAllStoriesOfProject = (
  project_id: string,
  search: string = '',
) => {
  return api.get(`/projects/${project_id}/stories`, {
    params: { search },
  });
};

export const getStoryById = (story_id: string) => {
  return api.get(`/stories/${story_id}`);
};

export const updateStoryStatus = (
  status: string,
  story_id: number | string,
) => {
  return api.put(`/stories/${story_id}/status`, { story_status: status, story_id: story_id });
};

export const getActivityLogs = (story_id: string) => {
  return api.get(`/stories/${story_id}/activity-logs`);
};

export const deleteStory = (story_id: number) => {
  return api.delete(`/stories/${story_id}`);
};

export const getAllExecutionOutputs = (id: string) => {
  return api.get(`/stories/${id}/execution-outputs`);
};

export const getProjectPullRequests = (project_id: string, status: string) => {
  return api.get(`/projects/${project_id}/pull-requests`, { params: { status } });
};

export const commentRebuildStory = (payload: CommentReBuildPayload) => {
  return api.post(`/pull-requests/${payload.pull_request_id}/comment`, payload);
};

export const mergePullRequest = (pull_request_id: number) => {
  return api.post(`/pull-requests/${pull_request_id}/merge`, { pull_request_id: pull_request_id });
};

export const getCommitsPullRequest = (pr_id: number) => {
  return api.get(`/pull-requests/${pr_id}/commits`);
};

export const getPullRequestDiff = (pr_id: number) => {
  return api.get(`/pull-requests/${pr_id}/diff`);
};

// Model APIs
export const getLLMAPIKeys = (organisation_id: string) => {
  return api.get(`/llm_api_key/${organisation_id}`);
};

export const createOrUpdateLLMAPIKey = (
  payload: CreateOrUpdateLLMAPIKeyPayload,
) => {
  return api.post(`/llm_api_key`, payload);
};
