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
}

export interface BrowserProps {
  url: string;
}
