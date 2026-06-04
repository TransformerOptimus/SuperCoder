import { useState, useEffect, useMemo } from "react";
import { parseDiff } from "react-diff-view";
import { Segmented } from "antd";
import { Rows3, Columns2, ChevronsDownUp, ChevronsUpDown } from "lucide-react";
import { useAppStore } from "@/store";
import { agentTauriService } from "@/services/agentTauriService";
import DiffReviewHeader from "./components/DiffReviewHeader/DiffReviewHeader";
import DiffFileView from "./components/DiffView/DiffFileView";
import styles from "./DiffReviewPanel.module.css";

function filePath(file: any): string {
  const isDel = !file.newPath || file.newPath === "/dev/null";
  return isDel ? file.oldPath : file.newPath;
}

function countAll(files: any[]): { adds: number; dels: number } {
  let adds = 0;
  let dels = 0;
  for (const f of files) {
    for (const h of f.hunks) {
      for (const c of h.changes) {
        if (c.type === "insert") adds++;
        else if (c.type === "delete") dels++;
      }
    }
  }
  return { adds, dels };
}

export default function DiffReviewPanel() {
  const activeAgentThreadId = useAppStore((s) => s.activeAgentThreadId);
  const agentThreads = useAppStore((s) => s.agentThreads);
  const expandedArtifactId = useAppStore((s) => s.expandedArtifactId);
  const closeDiffReview = useAppStore((s) => s.closeDiffReview);
  const showTerminal = useAppStore((s) => s.showTerminal);
  const toggleTerminal = useAppStore((s) => s.toggleTerminal);
  const toggleFileExplorer = useAppStore((s) => s.toggleFileExplorer);

  const [loadingDiff, setLoadingDiff] = useState(false);
  const [rawDiff, setRawDiff] = useState<string>("");
  const [viewType, setViewType] = useState<"unified" | "split">("unified");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const thread = activeAgentThreadId ? agentThreads[activeAgentThreadId] : null;

  // Load the cumulative working-tree diff whenever the panel opens or changes.
  useEffect(() => {
    if (!activeAgentThreadId || !expandedArtifactId) return;
    let cancelled = false;
    setLoadingDiff(true);
    agentTauriService
      .getFullDiff(activeAgentThreadId)
      .then((result) => {
        if (!cancelled) setRawDiff(result.diff || "");
      })
      .catch((err) => {
        if (!cancelled) {
          console.error("[DiffReviewPanel] Failed to load diff:", err);
          setRawDiff("");
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingDiff(false);
      });
    return () => {
      cancelled = true;
    };
  }, [activeAgentThreadId, expandedArtifactId, thread?.total_additions, thread?.total_deletions]);

  const files = useMemo(() => {
    if (!rawDiff.trim()) return [];
    try {
      // Drop entries with no actual line changes (empty hunks) — e.g. intent-to-add
      // placeholders or mode-only changes that render as a noisy "+0 -0" file.
      return parseDiff(rawDiff).filter((f: any) => f.hunks && f.hunks.length > 0);
    } catch (err) {
      console.error("[DiffReviewPanel] parseDiff failed:", err);
      return [];
    }
  }, [rawDiff]);

  const { adds, dels } = useMemo(() => countAll(files), [files]);

  if (!thread || !expandedArtifactId) return null;

  const collapseAll = () => setCollapsed(new Set(files.map(filePath)));
  const expandAll = () => setCollapsed(new Set());

  return (
    <div className={styles.panel}>
      <DiffReviewHeader
        artifactName="Code Changes"
        folderPath={thread.folder_path}
        branch={thread.branch}
        totalAdditions={adds}
        totalDeletions={dels}
        filesChanged={files.length}
        taskSummary={thread.task_summary}
        onClose={closeDiffReview}
        onToggleTerminal={toggleTerminal}
        showTerminal={showTerminal}
        onToggleFileExplorer={toggleFileExplorer}
      />

      <div className={styles.stats_row}>
        <span>
          {files.length} file{files.length !== 1 ? "s" : ""} changed
        </span>
        <span className={styles.stat_add}>+{adds}</span>
        <span className={styles.stat_del}>-{dels}</span>
        <div style={{ flex: 1 }} />
        <Segmented
          size="small"
          value={viewType === "split" ? "Split" : "Unified"}
          onChange={(v) => setViewType(v === "Split" ? "split" : "unified")}
          options={[
            { label: <span className="flex items-center gap-1"><Rows3 size={12} /> Unified</span>, value: "Unified" },
            { label: <span className="flex items-center gap-1"><Columns2 size={12} /> Split</span>, value: "Split" },
          ]}
        />
        <button onClick={collapseAll} className={styles.tool_btn} title="Collapse all">
          <ChevronsDownUp className="w-3.5 h-3.5" />
        </button>
        <button onClick={expandAll} className={styles.tool_btn} title="Expand all">
          <ChevronsUpDown className="w-3.5 h-3.5" />
        </button>
      </div>

      {loadingDiff ? (
        <div className={`${styles.body} ${styles.body_loading}`}>
          <span className={styles.loading_text}>Loading diff…</span>
        </div>
      ) : files.length === 0 ? (
        <div className={`${styles.body} ${styles.body_loading}`}>
          <span className={styles.loading_text}>No changes</span>
        </div>
      ) : (
        <div className={styles.body} style={{ display: "flex", flexDirection: "column", gap: 12, padding: 12 }}>
          {files.map((file) => {
            const path = filePath(file);
            return (
              <DiffFileView
                key={path + (file.oldRevision ?? "")}
                file={file}
                viewType={viewType}
                expanded={!collapsed.has(path)}
                onToggle={() =>
                  setCollapsed((prev) => {
                    const next = new Set(prev);
                    if (next.has(path)) next.delete(path);
                    else next.add(path);
                    return next;
                  })
                }
              />
            );
          })}
        </div>
      )}
    </div>
  );
}
