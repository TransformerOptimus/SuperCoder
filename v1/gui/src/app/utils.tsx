import {
  getAllProject,
  getAllStoriesOfProject,
  getLLMAPIKeys,
  getProjectPullRequests,
  logoutUser,
} from '@/api/DashboardService';
import { ProjectTypes } from '../../types/projectsTypes';
import toast from 'react-hot-toast';
import { storyActions, storyStatus } from '@/app/constants/BoardConstants';
import { Servers } from '@/app/constants/UtilsConstants';
import { StoryInReviewIssue } from '../../types/storyTypes';

export const logout = async () => {
  await logoutUser();
  if (typeof window !== 'undefined') {
    localStorage.removeItem('projectId');
    localStorage.removeItem('projectURL');
    localStorage.removeItem('projectURLFrontend');
    localStorage.removeItem('projectURLBackend');
    localStorage.removeItem('projectName');
    localStorage.removeItem('storyId');
    localStorage.removeItem('projectFrontendFramework');
  }
  window.location.replace('/');
};

export const getUsernameInitials = (username: string): string => {
  const nameParts = username.split(' ');
  const initials =
    nameParts?.length >= 2
      ? `${nameParts[0][0]}${nameParts[nameParts.length - 1][0]}`
      : username[0];
  return initials.toUpperCase();
};

export const handleStoryStatus = (status: string) => {
  const storyStatus = {
    TODO: { text: 'To Do', color: 'grey' },
    IN_PROGRESS: { text: 'In Progress', color: 'purple' },
    IN_REVIEW: { text: 'In Review', color: 'yellow' },
    DONE: { text: 'Done', color: 'green' },
    MAX_LOOP_ITERATION_REACHED: { text: 'In Review', color: 'yellow' },
  };

  return storyStatus[status] || { text: 'Default ', color: 'grey' };
};

export async function toGetProjectPullRequests(setter, status: string = 'ALL') {
  try {
    const id = localStorage.getItem('projectId');
    const response = await getProjectPullRequests(id, status);
    if (response) {
      const data = response.data;
      setter(data.pull_requests);
    }
  } catch (error) {
    console.error('Error file fetching pull requests: ', error);
  }
}

export async function handleInProgressStoryStatus(
  setOpenSetupModelModal,
  numberOfStoriesInProgress: number,
  toUpdateStoryStatus,
  id: string = Servers.BACKEND,
) {
  try {
    const modelNotAdded = await checkModelNotAdded(id);
    if (modelNotAdded) {
      setOpenSetupModelModal(true);
      return false;
    }
    if (numberOfStoriesInProgress >= 1) {
      toast.error('Cannot have two stories simultaneously In Progress', {
        style: {
          maxWidth: 'none',
          whiteSpace: 'nowrap',
        },
      });
      return false;
    }

    if (typeof window !== 'undefined') {
      toUpdateStoryStatus(storyStatus.IN_PROGRESS).then().catch();
      return true;
    }
  } catch (error) {
    console.error('Error while changing status: ' + error);
    return false;
  }
}

export async function toGetAllStoriesOfProjectUtils(
  setter,
  search = '',
  type = Servers.BACKEND,
) {
  try {
    const project_id = localStorage.getItem('projectId');
    const response = await getAllStoriesOfProject(project_id, search, type);
    if (response) {
      const data = response.data;
      setter(data.stories);
    }
  } catch (error) {
    console.error('Error while fetching all stories of project:: ', error);
  }
}

export async function toGetAllProjects(setter) {
  try {
    const response = await getAllProject();
    if (response) {
      const data = response.data;
      setter(data);
    }
  } catch (error) {
    console.error('Error while fetching all project:: ', error);
  }
}

export function formatTimeAgo(timestamp: string): string {
  const now = new Date();
  const past = new Date(timestamp);
  const diffInSeconds = Math.floor((now.getTime() - past.getTime()) / 1000);

  const units = [
    { name: 'y', seconds: 60 * 60 * 24 * 365 },
    { name: 'mo', seconds: 60 * 60 * 24 * 30 },
    { name: 'd', seconds: 60 * 60 * 24 },
    { name: 'h', seconds: 60 * 60 },
    { name: 'm', seconds: 60 },
    { name: 's', seconds: 1 },
  ];

  for (const unit of units) {
    const interval = Math.floor(diffInSeconds / unit.seconds);
    if (interval >= 1) {
      return `${interval}${unit.name} ago`;
    }
  }

  return 'just now';
}

export function setProjectDetails(project: ProjectTypes) {
  localStorage.setItem('projectFramework', project.project_framework);
  localStorage.setItem(
    'projectFrontendFramework',
    project.project_frontend_framework,
  );
  localStorage.setItem('projectId', project.project_id.toString());
  localStorage.setItem('projectURL', project.project_url);
  localStorage.setItem('projectURLFrontend', project.project_frontend_url);
  localStorage.setItem('projectURLBackend', project.project_backend_url);
  localStorage.setItem('projectName', project.project_name);
}

export async function checkModelNotAdded(id: string) {
  try {
    const response = await getLLMAPIKeys();
    if (response) {
      const data = response.data;
      if (Array.isArray(data)) {
        if (id === Servers.FRONTEND)
          return data.some((model) => model.api_key === '');
        else return data.every((model) => model.api_key === '');
      }
    }
  } catch (error) {
    console.error('Error while fetching LLM API Keys: ', error);
    return true;
  }
}

export function validateEmail(email: string) {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailRegex.test(email);
}

export function handleStoryInReviewIssue(data: {
  story: { reason: any };
}): StoryInReviewIssue {
  let issueTitle = '';
  let issueDescription = '';
  const actions = [];

  switch (data.story.reason) {
    case storyStatus.MAX_LOOP_ITERATIONS:
      issueTitle = 'Action Needed: Maximum number of iterations reached';
      issueDescription =
        'The story execution in the workbench has exceeded the maximum allowed iterations. You can update the story details and re-build it.';
      actions.push(
        { label: 'Re-Build', link: storyActions.REBUILD },
        {
          label: 'Get Help',
          link: 'https://discord.com/invite/dXbRe5BHJC',
        },
      );
      break;
    case storyStatus.LLM_KEY_NOT_FOUND:
      issueTitle = 'Action Needed: LLM API Key Configuration Error';
      issueDescription =
        'There is an issue with the LLM API Key configuration, which may involve an invalid or expired API key. Please verify the API Key settings and update them to continue.';
      actions.push(
        { label: 'Re-Build', link: storyActions.REBUILD },
        { label: 'Go to Settings', link: '/settings' },
      );
      break;
  }

  return {
    title: issueTitle,
    description: issueDescription,
    actions,
  };
}
