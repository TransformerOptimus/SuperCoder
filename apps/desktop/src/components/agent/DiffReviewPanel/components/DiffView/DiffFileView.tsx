import { useMemo } from "react";
import { Diff, Hunk, tokenize } from "react-diff-view";
import { ChevronDown, ChevronRight, FileCode } from "lucide-react";
import "react-diff-view/style/index.css";
import refractor, { languageForPath } from "./refractorSetup";
import styles from "./DiffFileView.module.css";

interface Props {
  file: any; // react-diff-view parsed FileData
  viewType: "unified" | "split";
  expanded: boolean;
  onToggle: () => void;
}

function displayPath(file: any): string {
  const isNew = !file.oldPath || file.oldPath === "/dev/null";
  const isDel = !file.newPath || file.newPath === "/dev/null";
  if (isDel) return file.oldPath;
  if (isNew || !file.oldPath || file.oldPath === file.newPath) return file.newPath;
  return `${file.oldPath} → ${file.newPath}`;
}

function countChanges(hunks: any[]): { adds: number; dels: number } {
  let adds = 0;
  let dels = 0;
  for (const h of hunks) {
    for (const c of h.changes) {
      if (c.type === "insert") adds++;
      else if (c.type === "delete") dels++;
    }
  }
  return { adds, dels };
}

export default function DiffFileView({ file, viewType, expanded, onToggle }: Props) {
  const language = languageForPath(file.newPath || file.oldPath);

  const tokens = useMemo(() => {
    if (!language) return undefined;
    try {
      return tokenize(file.hunks, { highlight: true, refractor, language });
    } catch {
      return undefined;
    }
  }, [file.hunks, language]);

  const { adds, dels } = useMemo(() => countChanges(file.hunks), [file.hunks]);

  return (
    <div className={styles.card}>
      <div className={styles.header} onClick={onToggle}>
        {expanded ? (
          <ChevronDown className="w-3.5 h-3.5 text-[var(--text-secondary)] shrink-0" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-[var(--text-secondary)] shrink-0" />
        )}
        <FileCode className="w-4 h-4 text-gray-500 shrink-0" />
        <span className={styles.path} title={displayPath(file)}>
          {displayPath(file)}
        </span>
        <span className={styles.statAdd}>+{adds}</span>
        <span className={styles.statDel}>-{dels}</span>
      </div>
      {expanded && (
        <div className={styles.body}>
          <Diff key={viewType} viewType={viewType} diffType={file.type} hunks={file.hunks} tokens={tokens}>
            {(hunks: any[]) => hunks.map((hunk) => <Hunk key={hunk.content} hunk={hunk} />)}
          </Diff>
        </div>
      )}
    </div>
  );
}
