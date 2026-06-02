import { FolderCode, FilePlus, ChevronDown, Code2, SquareTerminal, FolderOpen } from "lucide-react";
import { Dropdown, Button } from "antd";
import { invoke } from "@tauri-apps/api/core";
import type { ThreadPanelHeaderProps } from "./types";
import styles from "./ThreadPanelHeader.module.css";
import { projectDisplayName } from "@/utils/projectDisplayName";
import GitActionsDropdown from "../../../DiffReviewPanel/components/GitActions/GitActionsDropdown";

export default function ThreadPanelHeader({
  taskSummary,
  folderPath,
  branch,
  filesChanged,
  totalAdditions,
  totalDeletions,
  onExpandDiff,
}: ThreadPanelHeaderProps) {
  const folderName = projectDisplayName(folderPath);

  const openMenuItems = [
    { key: "vscode", label: "Open in VS Code", icon: <Code2 className="w-3.5 h-3.5" />, onClick: () => invoke("open_in_vscode", { path: folderPath }).catch(() => {}) },
    { key: "terminal", label: "Open in Terminal", icon: <SquareTerminal className="w-3.5 h-3.5" />, onClick: () => invoke("open_in_terminal", { path: folderPath }).catch(() => {}) },
    { key: "finder", label: "Reveal in Finder", icon: <FolderOpen className="w-3.5 h-3.5" />, onClick: () => invoke("open_in_finder", { path: folderPath }).catch(() => {}) },
  ];

  return (
    <div className={styles.header}>
      <div className="flex items-center gap-1.5 min-w-0" title={folderPath}>
        <FolderCode className="w-3.5 h-3.5 text-gray-400 shrink-0" />
        <span className="text-[13px] text-[var(--text-secondary)] truncate">{folderName}</span>
        {taskSummary && (
          <>
            <span className="text-[13px] text-gray-400 shrink-0">/</span>
            <span className="text-[13px] font-medium text-[var(--text-primary)] truncate">{taskSummary}</span>
          </>
        )}
      </div>

      <div className="flex-1" />

      <Dropdown trigger={["click"]} menu={{ items: openMenuItems }}>
        <Button className="secondary_small">
          <span className="flex items-center gap-1">
            Open <ChevronDown className="w-3 h-3" />
          </span>
        </Button>
      </Dropdown>

      {folderPath && (
        <GitActionsDropdown
          repoPath={folderPath}
          branch={branch || "main"}
          filesChanged={filesChanged}
          totalAdditions={totalAdditions}
          totalDeletions={totalDeletions}
          taskSummary={taskSummary}
        />
      )}

      <div
        className={styles.stats}
        onClick={onExpandDiff}
        style={onExpandDiff ? { cursor: "pointer" } : undefined}
        title={onExpandDiff ? "View code changes" : undefined}
      >
        <FilePlus className="w-3.5 h-3.5 text-gray-500 shrink-0" />
        <span className={styles.stat_add}>+{totalAdditions}</span>
        <span className={styles.stat_del}>-{totalDeletions}</span>
      </div>
    </div>
  );
}
