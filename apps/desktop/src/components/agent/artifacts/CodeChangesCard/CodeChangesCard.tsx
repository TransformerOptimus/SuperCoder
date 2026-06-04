import { Card } from "antd";
import { Code, Maximize2 } from "lucide-react";
import type { CodeChangesCardProps } from "./types";

export default function CodeChangesCard({ artifact, onExpand }: CodeChangesCardProps) {
  const fileCount = artifact.files.length;

  return (
    <Card
      size="small"
      hoverable={!!onExpand}
      onClick={() => onExpand?.(artifact.id)}
      className="cursor-pointer"
    >
      <div className="flex items-center gap-2">
        <Code className="w-4 h-4 text-[var(--text-secondary)] shrink-0" />
        <span className="text-xs text-[var(--text-secondary)]">
          {fileCount} file{fileCount !== 1 ? "s" : ""} changed
        </span>
        <div className="flex items-center gap-1.5 ml-auto">
          <span className="text-xs font-mono text-[var(--diff-add-color)]">+{artifact.total_additions}</span>
          <span className="text-xs font-mono text-[var(--diff-del-color)]">-{artifact.total_deletions}</span>
        </div>
        {onExpand && (
          <Maximize2 className="w-4 h-4 text-[var(--text-secondary)] opacity-60 hover:opacity-100 transition-opacity" />
        )}
      </div>
    </Card>
  );
}
