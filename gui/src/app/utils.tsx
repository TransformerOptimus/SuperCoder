import {
  getAllProject,
  getAllStoriesOfProject,
  getLLMAPIKeys,
  getProjectPullRequests,
} from '@/api/DashboardService';
import { removeCookie } from '@/utils/CookieUtils';
import { ProjectTypes } from '../../types/projectsTypes';
import toast from 'react-hot-toast';
import { storyStatus } from '@/app/constants/BoardConstants';

export const logout = () => {
  if (typeof window !== 'undefined') {
    removeCookie('accessToken');
    localStorage.removeItem('accessToken');
    localStorage.removeItem('userId');
    localStorage.removeItem('userName');
    localStorage.removeItem('userEmail');
    localStorage.removeItem('projectId');
    localStorage.removeItem('projectURL');
    localStorage.removeItem('projectURLFrontend');
    localStorage.removeItem('projectURLBackend');
    localStorage.removeItem('projectName');
    localStorage.removeItem('storyId');
    localStorage.removeItem('organisationId');
    localStorage.removeItem('projectFrontendFramework');
  }

  window.location.replace('/');
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
  number_of_stories_in_progress: number,
  toUpdateStoryStatus,
) {
  try {
    const modelNotAdded = await checkModelNotAdded();
    if (modelNotAdded) {
      setOpenSetupModelModal(true);
      return false;
    }
    if (number_of_stories_in_progress >= 1) {
      toast.error('Cannot have two stories simultaneously In Progress', {
        style: {
          border: '1px solid #713200',
          padding: '16px',
          color: '#713200',
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
    console.error(error);
    return false;
  }
}

export async function toGetAllStoriesOfProjectUtils(
  setter,
  search = '',
  type = 'backend',
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

export async function checkModelNotAdded() {
  try {
    const organisation_id = localStorage.getItem('organisationId');
    const response = await getLLMAPIKeys(organisation_id);
    if (response) {
      const data = response.data;
      if (Array.isArray(data)) {
        return data.every((model) => model.api_key === '');
      }
      return true;
    }
  } catch (error) {
    console.error('Error while fetching LLM API Keys: ', error);
    return true;
  }
}
