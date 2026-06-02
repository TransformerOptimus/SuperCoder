export interface DiffReviewHeaderProps {
  artifactName: string;
  folderPath: string;
  branch: string;
  totalAdditions: number;
  totalDeletions: number;
  filesChanged: number;
  taskSummary?: string;
  onClose: () => void;
  onToggleTerminal?: () => void;
  showTerminal?: boolean;
  onToggleFileExplorer?: () => void;
}
