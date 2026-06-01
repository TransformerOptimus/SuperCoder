export interface PRListItems {
  pull_request_id: number;
  pull_request_description: string;
  pull_request_name: string;
  pull_request_number: number;
  created_on: string;
  merged_on?: string;
  closed_on?: string;
  total_comments: number;
  status: string;
}

export interface PRListProps {
  type: string;
  list: PRListItems[];
}

export interface PRDetailsProps {
  props: [];
  id?: number;
  pr: PRListItems;
}

export interface Commit {
  date: string;
  message: string;
  author: string;
  hash: string;
}

export interface CommentReBuildPayload {
  pull_request_id: number;
  comment: string;
}

export interface CommentReBuildDesignStoryPayload {
  story_id: number;
  comment: string;
}

export interface CommitItems {
  title: string;
  commiter: string;
  time: string;
  date: string;
  sha: string;
}

export interface CommitLogsProps {
  commits: CommitItems[];
}

export interface FilesChangedProps {
  diff: string;
}

export interface CreatePullRequestPayload {
  project_id: number;
  title: string;
  description: string;
}
