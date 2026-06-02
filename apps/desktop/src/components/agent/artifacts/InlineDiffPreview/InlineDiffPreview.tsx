import { Maximize2, Code } from "lucide-react";
import type { CodeChangesArtifact } from "@/types/agent";
import styles from "./InlineDiffPreview.module.css";

const MAX_FILES = 5;

interface InlineDiffPreviewProps {
  artifact: CodeChangesArtifact;
  maxLines?: number;
  onExpand?: (id: string) => void;
}

export default function InlineDiffPreview({
  artifact,
  maxLines = 6,
  onExpand,
}: InlineDiffPreviewProps) {
  const handleClick = () => {
    onExpand?.(artifact.id);
  };

  return (
    <div className={styles.container} onClick={handleClick}>
      <div className={styles.header}>
        <span className={styles.name}>{artifact.name}</span>
        <div className={styles.stats}>
          <span className={styles.stat_add}>+{artifact.total_additions}</span>
          <span className={styles.stat_del}>-{artifact.total_deletions}</span>
        </div>
        <Maximize2 className={`w-3.5 h-3.5 text-gray-500 ${styles.expand_icon}`} />
      </div>

      {artifact.files.slice(0, MAX_FILES).map((file) => {
        const previewLines = file.hunks
          .flatMap((h) => h.lines)
          .filter((l) => l.type !== "context")
          .slice(0, maxLines);

        return (
          <div key={file.file_path} className={styles.file_section}>
            <div className={styles.file_header}>
              <Code className="w-3.5 h-3.5 text-gray-500 shrink-0" />
              <span className={styles.file_path}>{file.file_path}</span>
              <span className={styles.file_stat_add}>+{file.additions}</span>
              <span className={styles.file_stat_del}>-{file.deletions}</span>
            </div>
            <div className={styles.diff_preview}>
              {previewLines.map((line, i) => (
                <div
                  key={i}
                  className={
                    line.type === "add"
                      ? styles.line_add
                      : line.type === "delete"
                        ? styles.line_delete
                        : styles.line_context
                  }
                >
                  <span className={styles.line_prefix}>
                    {line.type === "add" ? "+" : line.type === "delete" ? "-" : " "}
                  </span>
                  <span>{line.content}</span>
                </div>
              ))}
            </div>
          </div>
        );
      })}
      {artifact.files.length > MAX_FILES && (
        <div className={styles.file_section}>
          <div className={styles.file_header} style={{ opacity: 0.6 }}>
            <span className={styles.file_path}>
              and {artifact.files.length - MAX_FILES} more file{artifact.files.length - MAX_FILES !== 1 ? 's' : ''}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
