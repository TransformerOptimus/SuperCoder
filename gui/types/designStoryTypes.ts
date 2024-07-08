export interface DesignStoryItem {
  id: number;
  status: string;
  title: string;
  input_file_url: string;
  created_on: string;
  review_viewed: boolean;
}

export interface CreateEditDesignStoryProps {
  id: string;
  top: string;
  close: () => void;
  toGetAllDesignStoriesOfProject?: () => void;
}

export interface DesignStoryDetailsProps {
  id: string;
  top: string;
  close: () => void;
  toGetAllDesignStoriesOfProject: () => void;
  setOpenSetupModelModal: React.Dispatch<React.SetStateAction<boolean>>;
  number_of_stories_in_progress: number;
}

export interface CreateDesignStoryPayload {
  project_id: string;
  title: string;
  file: Blob;
  imageName: string;
}

export interface EditDesignStoryPayload {
  story_id: number;
  title: string;
  file: Blob;
  imageName: string;
}
