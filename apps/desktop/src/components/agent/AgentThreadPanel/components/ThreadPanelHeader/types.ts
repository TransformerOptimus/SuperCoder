export interface ThreadPanelHeaderProps {
  taskSummary: string;
  folderPath: string;
  branch: string;
  filesChanged: number;
  totalAdditions: number;
  totalDeletions: number;
  onExpandDiff?: () => void;
}
