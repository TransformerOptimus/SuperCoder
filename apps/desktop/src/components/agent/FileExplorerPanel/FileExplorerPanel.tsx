import { useState, useMemo } from "react";
import { Tree, Input } from "antd";
import { X } from "lucide-react";
import { useAppStore } from "@/store";
import type { FileDiff } from "@/types/agent";
import type { DataNode } from "antd/es/tree";
import styles from "./FileExplorerPanel.module.css";

/** Build a directory tree from flat file paths */
function buildTree(files: FileDiff[]): DataNode[] {
  const root: Record<string, any> = {};

  for (const file of files) {
    const parts = file.file_path.split("/");
    let current = root;
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      if (!current[part]) {
        current[part] = i === parts.length - 1 ? { __file: file } : {};
      }
      current = current[part];
    }
  }

  function toNodes(obj: Record<string, any>, prefix: string): DataNode[] {
    return Object.keys(obj)
      .sort((a, b) => {
        const aIsDir = !obj[a].__file;
        const bIsDir = !obj[b].__file;
        if (aIsDir !== bIsDir) return aIsDir ? -1 : 1;
        return a.localeCompare(b);
      })
      .map((key) => {
        const val = obj[key];
        const fullPath = prefix ? `${prefix}/${key}` : key;

        if (val.__file) {
          const file = val.__file as FileDiff;
          return {
            key: fullPath,
            isLeaf: true,
            title: (
              <div className="flex items-center gap-1.5 min-w-0">
                <span className="truncate flex-1">{key}</span>
                <span className="text-[10px] font-mono text-[var(--diff-add-color)] shrink-0">+{file.additions}</span>
                <span className="text-[10px] font-mono text-[var(--diff-del-color)] shrink-0">-{file.deletions}</span>
              </div>
            ),
          };
        }

        const children = toNodes(val, fullPath);
        // Collapse single-child directories: src/components → src/components
        if (children.length === 1 && !children[0].isLeaf) {
          const child = children[0];
          const childKey = child.key as string;
          const combinedName = `${key}/${childKey.slice(fullPath.length + 1)}`;
          return { ...child, key: childKey, title: <span>{combinedName}</span> };
        }

        return {
          key: fullPath,
          title: <span>{key}</span>,
          children,
        };
      });
  }

  return toNodes(root, "");
}

function getAllDirKeys(nodes: DataNode[]): string[] {
  const keys: string[] = [];
  for (const node of nodes) {
    if (node.children && node.children.length > 0) {
      keys.push(node.key as string);
      keys.push(...getAllDirKeys(node.children));
    }
  }
  return keys;
}

export default function FileExplorerPanel() {
  const toggleFileExplorer = useAppStore((s) => s.toggleFileExplorer);
  const expandedArtifactId = useAppStore((s) => s.expandedArtifactId);
  const activeAgentThreadId = useAppStore((s) => s.activeAgentThreadId);
  const agentThreads = useAppStore((s) => s.agentThreads);
  const [search, setSearch] = useState("");

  const changedFiles: FileDiff[] = useMemo(() => {
    if (!activeAgentThreadId || !expandedArtifactId) return [];
    const thread = agentThreads[activeAgentThreadId];
    if (!thread) return [];
    for (const msg of thread.messages) {
      const artifact = msg.artifacts.find((a) => a.id === expandedArtifactId);
      if (artifact && artifact.type === "code_changes") {
        return (artifact as any).files ?? [];
      }
    }
    return [];
  }, [activeAgentThreadId, expandedArtifactId, agentThreads]);

  const filtered = useMemo(() => {
    if (!search) return changedFiles;
    const q = search.toLowerCase();
    return changedFiles.filter((f) => f.file_path.toLowerCase().includes(q));
  }, [changedFiles, search]);

  const treeData = useMemo(() => buildTree(filtered), [filtered]);
  const expandedKeys = useMemo(() => getAllDirKeys(treeData), [treeData]);

  const handleSelect = (keys: any) => {
    const key = keys[0] as string;
    if (!key) return;
    const el = document.querySelector(`[data-file-path="${CSS.escape(key)}"]`);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  };

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <span className={styles.title}>Changed Files</span>
        <span className="text-[11px] text-[var(--text-secondary)]">{changedFiles.length}</span>
        <div className="flex-1" />
        <X
          className={`w-3.5 h-3.5 cursor-pointer ${styles.close_btn}`}
          onClick={toggleFileExplorer}
        />
      </div>
      <div className={styles.search_area}>
        <Input
          className="input_small"
          placeholder="Filter files..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          allowClear
        />
      </div>
      <div className={styles.tree_area}>
        {filtered.length === 0 ? (
          <span className={styles.placeholder_text}>No changed files</span>
        ) : (
          <Tree
            treeData={treeData}
            defaultExpandedKeys={expandedKeys}
            showLine
            showIcon={false}
            blockNode
            onSelect={handleSelect}
          />
        )}
      </div>
    </div>
  );
}
