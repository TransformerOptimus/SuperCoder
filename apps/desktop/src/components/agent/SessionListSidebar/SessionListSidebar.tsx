import { useEffect, useState, useCallback, useMemo, useRef } from "react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { Empty, Spin, Tooltip } from "antd";
import {
  Plus,
  MessageCircle,
  Map as MapIcon,
  Code,
  Pencil,
  ChevronDown,
  ChevronRight,
  Settings as SettingsIcon,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "@/store";
import { agentTauriService } from "@/services/agentTauriService";
import { themedMessage } from "@/providers/AntDThemeProvider";
import { displayToAgentMessage } from "@/utils/agentMessageAdapter";
import { getAvatarColor } from "@/utils/avatarUtils";
import type { SessionRow } from "@/types/agent";

const MODE_ICON: Record<string, typeof MessageCircle> = {
  ask: MessageCircle,
  plan: MapIcon,
  coding: Code,
};

function folderName(path: string): string {
  return path.split("/").filter(Boolean).pop() || path;
}

export default function SessionListSidebar() {
  const navigate = useNavigate();
  const sessions = useAppStore((s) => s.sessions);
  const sessionsLoading = useAppStore((s) => s.sessionsLoading);
  const loadSessions = useAppStore((s) => s.loadSessions);
  const activeAgentThreadId = useAppStore((s) => s.activeAgentThreadId);
  const activeSessionIds = useAppStore((s) => s.activeSessionIds);
  const pendingApprovals = useAppStore((s) => s.pendingApprovals);
  const collapsed = useAppStore((s) => s.sidebarCollapsed);

  // Session ids that currently have a tool-approval waiting on the user.
  const approvalSessionIds = useMemo(
    () => new Set(Object.values(pendingApprovals).map((a) => a.threadId)),
    [pendingApprovals],
  );
  const toggleSidebar = useAppStore((s) => s.toggleSidebar);

  const [collapsedFolders, setCollapsedFolders] = useState<Set<string>>(new Set());
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editingText, setEditingText] = useState("");
  const editRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    loadSessions();
  }, [loadSessions]);

  const groups = useMemo(() => {
    const map = new Map<string, SessionRow[]>();
    for (const s of sessions) {
      const arr = map.get(s.folder);
      if (arr) arr.push(s);
      else map.set(s.folder, [s]);
    }
    return Array.from(map.entries());
  }, [sessions]);

  const toggleFolder = useCallback((folder: string) => {
    setCollapsedFolders((prev) => {
      const next = new Set(prev);
      if (next.has(folder)) next.delete(folder);
      else next.add(folder);
      return next;
    });
  }, []);

  const openSession = useCallback(async (s: SessionRow) => {
    const store = useAppStore.getState();
    store.upsertAgentThread({
      id: s.id,
      agent_id: "",
      task_summary: s.title || folderName(s.folder),
      folder_path: s.folder,
      branch: store.agentBranch ?? "main",
      status: s.status === "error" ? "error" : s.status === "active" ? "active" : "completed",
      is_coding_session: s.mode === "coding",
      total_additions: 0,
      total_deletions: 0,
      checkpoints: [],
      selectedDiffTurn: null,
      messages: [],
      created_at: s.created_at,
      updated_at: s.updated_at,
    });
    store.openAgentThread(s.id);
    store.setAgentThreadLoading(s.id, true);
    try {
      const msgs = await agentTauriService.getMessages(s.id);
      store.replaceThreadMessages(s.id, msgs.map(displayToAgentMessage));
    } catch (err) {
      console.error("[SessionListSidebar] Failed to load messages:", err);
    } finally {
      store.setAgentThreadLoading(s.id, false);
    }
  }, []);

  const createInFolder = useCallback(
    async (folder: string) => {
      try {
        const session = await agentTauriService.createSession(folder);
        useAppStore.getState().upsertSession(session);
        await openSession(session);
      } catch (err) {
        console.error("[SessionListSidebar] create failed:", err);
        themedMessage.error("Failed to create session");
      }
    },
    [openSession],
  );

  const handleNewSession = useCallback(async () => {
    const selected = await openDialog({ directory: true, multiple: false, title: "Select a project folder" });
    if (selected) await createInFolder(selected as string);
  }, [createInFolder]);

  // ── Inline rename ───────────────────────────────────────────────────────
  const startRename = useCallback((s: SessionRow) => {
    setEditingId(s.id);
    setEditingText(s.title || folderName(s.folder));
    requestAnimationFrame(() => editRef.current?.select());
  }, []);

  const commitRename = useCallback(async () => {
    const id = editingId;
    const text = editingText.trim();
    setEditingId(null);
    if (!id || !text) return;
    const current = useAppStore.getState().sessions.find((x) => x.id === id);
    if (current && current.title === text) return;
    useAppStore.getState().setSessionTitle(id, text);
    try {
      await agentTauriService.renameSession(id, text);
    } catch (err) {
      console.error("[SessionListSidebar] rename failed:", err);
    }
  }, [editingId, editingText]);

  // ── Collapsed rail ──────────────────────────────────────────────────────
  if (collapsed) {
    return (
      <div className="h-full flex flex-col items-center justify-between py-3 bg-white dark:bg-dark-bg border-r border-gray-200 dark:border-dark-border">
        <div className="flex flex-col items-center gap-3">
          <Tooltip title="New session" placement="right">
            <button onClick={handleNewSession} className="w-9 h-9 flex items-center justify-center rounded-lg text-white bg-blue-600 hover:bg-blue-700">
              <Plus className="w-4 h-4" />
            </button>
          </Tooltip>
          <div className="flex flex-col items-center gap-2 mt-1">
            {groups.slice(0, 8).map(([folder, items]) => {
              const hasActive = items.some((s) => s.id === activeAgentThreadId);
              const needsApproval = items.some((s) => approvalSessionIds.has(s.id));
              return (
                <Tooltip key={folder} title={needsApproval ? `${folderName(folder)} — needs approval` : folderName(folder)} placement="right">
                  <button
                    onClick={() => openSession(items[0])}
                    className={`relative w-8 h-8 flex items-center justify-center rounded-md text-white text-xs font-semibold ${hasActive ? "ring-2 ring-blue-500" : ""}`}
                    style={{ backgroundColor: getAvatarColor(folder) }}
                  >
                    {folderName(folder).charAt(0).toUpperCase()}
                    {needsApproval && (
                      <span className="absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-amber-500 ring-2 ring-white dark:ring-dark-bg animate-pulse" />
                    )}
                  </button>
                </Tooltip>
              );
            })}
          </div>
        </div>
        <div className="flex flex-col items-center gap-3">
          <Tooltip title="Settings" placement="right">
            <button onClick={() => navigate("/settings")} className="w-9 h-9 flex items-center justify-center rounded-lg text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-dark-hover">
              <SettingsIcon className="w-4 h-4" />
            </button>
          </Tooltip>
          <Tooltip title="Expand sidebar" placement="right">
            <button onClick={toggleSidebar} className="w-9 h-9 flex items-center justify-center rounded-lg text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-dark-hover">
              <PanelLeftOpen className="w-4 h-4" />
            </button>
          </Tooltip>
        </div>
      </div>
    );
  }

  // ── Expanded sidebar ────────────────────────────────────────────────────
  return (
    <div className="h-full flex flex-col bg-white dark:bg-dark-bg border-r border-gray-200 dark:border-dark-border">
      <div className="px-3 pt-3 pb-1">
        <button
          onClick={handleNewSession}
          className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-dark-hover"
        >
          <Plus className="w-4 h-4" /> New session
        </button>
      </div>

      <div className="px-4 pt-2 pb-1">
        <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">Sessions</span>
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {sessionsLoading && sessions.length === 0 ? (
          <div className="flex justify-center py-8">
            <Spin />
          </div>
        ) : groups.length === 0 ? (
          <div className="py-10">
            <Empty description="No sessions yet" />
          </div>
        ) : (
          groups.map(([folder, items]) => {
            const isFolderCollapsed = collapsedFolders.has(folder);
            return (
              <div key={folder} className="mb-1.5">
                {/* Folder header (collapsible + hover actions) */}
                <div
                  className="group flex items-center gap-1.5 px-1.5 py-1.5 rounded-md cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-hover/60"
                  onClick={() => toggleFolder(folder)}
                >
                  {isFolderCollapsed ? (
                    <ChevronRight className="w-3.5 h-3.5 text-gray-400 shrink-0" />
                  ) : (
                    <ChevronDown className="w-3.5 h-3.5 text-gray-400 shrink-0" />
                  )}
                  <span
                    className="w-5 h-5 flex items-center justify-center rounded text-white text-[10px] font-semibold shrink-0"
                    style={{ backgroundColor: getAvatarColor(folder) }}
                  >
                    {folderName(folder).charAt(0).toUpperCase()}
                  </span>
                  <span className="text-sm font-medium text-gray-800 dark:text-gray-200 truncate flex-1" title={folder}>
                    {folderName(folder)}
                  </span>
                  <Tooltip title="New session in this folder">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        createInFolder(folder);
                      }}
                      className="opacity-0 group-hover:opacity-100 p-0.5 rounded text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-200 dark:hover:bg-dark-hover transition-opacity"
                    >
                      <Plus className="w-3.5 h-3.5" />
                    </button>
                  </Tooltip>
                </div>

                {/* Sessions */}
                {!isFolderCollapsed && (
                  <div className="flex flex-col">
                    {items.map((s) => {
                      const Icon = MODE_ICON[s.mode] ?? MessageCircle;
                      const active = s.id === activeAgentThreadId;
                      const isEditing = editingId === s.id;
                      return (
                        <div
                          key={s.id}
                          className={`group flex items-center gap-2 ml-3 pl-3 pr-1.5 py-1.5 rounded-md border-l border-gray-100 dark:border-dark-border ${
                            active ? "bg-gray-100 dark:bg-dark-hover" : "hover:bg-gray-50 dark:hover:bg-dark-hover/60"
                          }`}
                        >
                          <Icon className="w-3.5 h-3.5 text-gray-400 shrink-0" />
                          {isEditing ? (
                            <input
                              ref={editRef}
                              value={editingText}
                              onChange={(e) => setEditingText(e.target.value)}
                              onBlur={commitRename}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") commitRename();
                                else if (e.key === "Escape") setEditingId(null);
                              }}
                              autoFocus
                              className="flex-1 min-w-0 text-[13px] bg-transparent outline-none border-b border-blue-400 text-gray-800 dark:text-gray-100"
                            />
                          ) : (
                            <button onClick={() => openSession(s)} className="flex-1 min-w-0 text-left">
                              <span className="text-[13px] text-gray-700 dark:text-gray-300 truncate block">
                                {s.title || "Untitled"}
                              </span>
                            </button>
                          )}
                          {!isEditing && approvalSessionIds.has(s.id) && (
                            <Tooltip title="Needs approval">
                              <span className="w-2 h-2 rounded-full bg-amber-500 shrink-0 animate-pulse" />
                            </Tooltip>
                          )}
                          {!isEditing && !approvalSessionIds.has(s.id) && !!activeSessionIds[s.id] && (
                            <span className="w-1.5 h-1.5 rounded-full bg-green-500 shrink-0" title="Running" />
                          )}
                          {!isEditing && (
                            <Tooltip title="Rename">
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  startRename(s);
                                }}
                                className="opacity-0 group-hover:opacity-100 p-0.5 rounded text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-opacity"
                              >
                                <Pencil className="w-3 h-3" />
                              </button>
                            </Tooltip>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>

      {/* Footer: settings + collapse */}
      <div className="flex items-center justify-between px-2 py-2 border-t border-gray-200 dark:border-dark-border">
        <button
          onClick={() => navigate("/settings")}
          className="flex items-center gap-2 px-2 py-1.5 rounded-md text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-dark-hover"
        >
          <SettingsIcon className="w-4 h-4" /> Settings
        </button>
        <button
          onClick={toggleSidebar}
          className="p-1.5 rounded-md text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-dark-hover"
          title="Collapse sidebar"
        >
          <PanelLeftClose className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}
