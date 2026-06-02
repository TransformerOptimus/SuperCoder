import { useMemo } from "react";
import { invoke } from "@tauri-apps/api/core";
import { Code, Terminal, FolderCode, FolderGit2, ChevronDown, X } from "lucide-react";
import CustomDropdown from "@/components/common/CustomDropdown/CustomDropdown";
import GitActionsDropdown from "../GitActions/GitActionsDropdown";
import type { CustomDropdownItem } from "@/components/common/CustomDropdown/types";
import type { DiffReviewHeaderProps } from "./types";
import styles from "./DiffReviewHeader.module.css";

export default function DiffReviewHeader({
  artifactName,
  folderPath,
  branch,
  totalAdditions,
  totalDeletions,
  filesChanged,
  taskSummary,
  onClose,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  onToggleTerminal: _onToggleTerminal,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  showTerminal: _showTerminal,
  onToggleFileExplorer,
}: DiffReviewHeaderProps) {
  const folderName = folderPath.split("/").pop() ?? folderPath;

  const iconClass = "w-4 h-4 text-gray-500";
  const openItems: CustomDropdownItem[] = useMemo(
    () => [
      {
        key: "vscode",
        icon: <Code className={iconClass} />,
        label: "Open in VS Code",
        onClick: async () => {
          try {
            await invoke("open_in_vscode", { path: folderPath });
          } catch (err) {
            console.error("[DiffReviewHeader] Failed to open VS Code:", err);
          }
        },
      },
      {
        key: "terminal",
        icon: <Terminal className={iconClass} />,
        label: "Open Terminal",
        onClick: async () => {
          try {
            await invoke("open_in_terminal", { path: folderPath });
          } catch (err) {
            console.error("[DiffReviewHeader] Failed to open Terminal:", err);
          }
        },
      },
      {
        key: "finder",
        icon: <FolderCode className={iconClass} />,
        label: "Show in Finder",
        onClick: async () => {
          try {
            await invoke("open_in_finder", { path: folderPath });
          } catch (err) {
            console.error("[DiffReviewHeader] Failed to show in Finder:", err);
          }
        },
      },
      {
        key: "file_explorer",
        icon: <FolderGit2 className={iconClass} />,
        label: "File Explorer",
        onClick: () => {
          onToggleFileExplorer?.();
        },
        dividerBefore: true,
      },
    ],
    [folderPath, onToggleFileExplorer],
  );

  return (
    <div className={styles.header}>
      <span className={styles.title}>{artifactName}</span>
      <div className={styles.folder_chip}>
        <FolderCode className="w-3 h-3 text-gray-500 shrink-0" />
        <span>{folderName}</span>
      </div>
      <div className="flex-1" />
      <CustomDropdown
        items={openItems}
        placement="bottomRight"
        trigger={
          <button className={styles.dropdown_btn}>
            Open
            <ChevronDown className="w-3 h-3" />
          </button>
        }
      />
      <GitActionsDropdown
        repoPath={folderPath}
        branch={branch}
        filesChanged={filesChanged}
        totalAdditions={totalAdditions}
        totalDeletions={totalDeletions}
        taskSummary={taskSummary}
      />
      <div className={styles.icon_group}>
        {/* TODO: Enable when terminal input is working
        <Terminal
          className={`w-4 h-4 cursor-pointer ${styles.icon_btn} ${showTerminal ? styles.icon_btn_active : ''}`}
          onClick={() => onToggleTerminal?.()}
          title="Toggle Terminal"
        />
        */}
        <X
          className={`w-4 h-4 cursor-pointer ${styles.icon_btn}`}
          onClick={onClose}
        />
      </div>
    </div>
  );
}
