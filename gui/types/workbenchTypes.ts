import { b } from '@nextui-org/slider/dist/use-slider-a94a4c83';

export interface StoryDetailsWorkbenchProps {
  id: string;
}

export interface StoryListItems {
  story_id: number;
  story_name: string;
}

export interface StoryList {
  IN_PROGRESS: StoryListItems[];
  DONE: StoryListItems[];
  IN_REVIEW: StoryListItems[];
}

export interface BrowserProps {
  url: string;
  status: boolean;
  showUrl?: boolean;
}

export interface ActivityItem {
  LogMessage: string;
  Type: string;
  CreatedAt: string;
}

export interface ActiveWorkbenchProps {
  storiesList: StoryList;
  storyType: string;
}

export interface BackendWorkbenchProps {
  activityLogs: ActivityItem[];
  selectedStoryId: string | number;
  status: boolean;
}

export interface DesignWorkbenchProps {
  activityLogs: ActivityItem[];
  selectedStoryId: string;
  executionInProcess: boolean;
}
