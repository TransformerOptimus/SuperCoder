import { Card } from "antd";
import type { FileArtifact } from "@/types/agent";

interface FileCardProps {
  artifact: FileArtifact;
}

export default function FileCard({ artifact }: FileCardProps) {
  return (
    <Card size="small">
      <div className="flex items-center gap-2">
        <span className="text-xs text-[var(--text-secondary)] truncate flex-1">
          {artifact.file_name}
        </span>
        {artifact.size != null && (
          <span className="text-xs text-[var(--text-secondary)] opacity-50">
            {(artifact.size / 1024).toFixed(1)} KB
          </span>
        )}
      </div>
    </Card>
  );
}
