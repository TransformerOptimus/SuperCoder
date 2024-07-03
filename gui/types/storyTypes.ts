export interface CreateEditStoryProps {
  id: string;
  top: string;
  close: () => void;
  toGetAllStoriesOfProject?: () => void;
  story_id?: string | number;
}

export interface InputSectionProps {
  id: string;
  label: string;
  type?: string;
  isTextArea?: boolean;
  placeholder?: string;
}

export interface DynamicInputSectionProps {
  id: string;
  label: string;
  inputs: string[];
  placeholder?: string;
  setInputs: React.Dispatch<React.SetStateAction<string[]>>;
  buttonText?: string;
}

export interface FormStoryPayload {
  story_id?: number | string;
  project_id: number;
  summary: string;
  description: string;
  test_cases: string[];
  instructions: string;
}

export interface OverviewContent {
  name?: string;
  summary?: string;
  description?: string;
}

export interface StoryOverview {
  overview: OverviewContent;
}

export interface StoryInstructions {
  instructions: string[];
}

export interface StoryTestCases {
  cases: string[];
}

export interface StoryDetailsAPIData {
  instructions: string;
  overview: OverviewContent;
  status: string;
  test_cases: string[];
}

export interface StoryDetailsProps {
  id: string;
  story_id: string | number;
  open_status?: boolean;
  tabCSS?: string;
  toGetAllStoriesOfProject?: () => void;
  close?: () => void;
  number_of_stories_in_progress?: number;
  setOpenSetupModelModal?: React.Dispatch<React.SetStateAction<boolean>>;
}

export interface SetupModelModalProps {
  openModal: boolean;
  setOpenModel: React.Dispatch<React.SetStateAction<boolean>>;
}
