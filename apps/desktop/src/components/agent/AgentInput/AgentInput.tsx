import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { open } from "@tauri-apps/plugin-dialog";
import { invoke } from "@tauri-apps/api/core";
import { X, FileText, Code, File as FileIcon, Bot, MessageCircle, Map, Plus, MoreHorizontal, CircleDashed } from "lucide-react";
import { useAppStore } from "@/store";
import { useAgentSend } from "@/hooks/useAgentSend";
import { saveDraft, loadDraft, clearDraft } from "@/utils/drafts";
import { agentTauriService } from "@/services/agentTauriService";
import InputShell from "@/components/common/InputShell/InputShell";
import ActionChip from "@/components/common/ActionChip/ActionChip";
import PermissionSettingsModal from "../PermissionSettingsModal/PermissionSettingsModal";
import SkillsDialog from "../SkillsDialog/SkillsDialog";
import SubagentsDialog from "../SubagentsDialog/SubagentsDialog";
import { Segmented, Progress, Tooltip, Popover } from "antd";
import { themedMessage } from "@/providers/AntDThemeProvider";
import type { PermissionLevel, SubagentListEntry } from "@/types/agentContract";
import type { Attachment } from "@/types/chat";
import type { WidthTier } from "@/components/common/InputShell/types";
import ModelPicker from "@/components/agent/ModelPicker/ModelPicker";
import type { AgentInputProps } from "./types";

const PERMISSION_LABELS: Record<PermissionLevel, string> = {
  AutoApproveAll: "Full auto",
  ApproveDestructive: "Balanced",
  ApproveEverything: "Ask all",
};

interface PendingAttachment {
  id: string;
  file: File;
  file_name: string;
  uploading: boolean;
  error?: string;
  uploaded?: Attachment;
}

function isImage(name: string): boolean {
  return /\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(name);
}

/** Infer an image MIME type from a filename. Files built from raw bytes have an
 * empty `type`, which the backend's `image/*` filter would drop. */
function mimeFromName(name: string): string {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  const map: Record<string, string> = {
    png: "image/png",
    jpg: "image/jpeg",
    jpeg: "image/jpeg",
    gif: "image/gif",
    webp: "image/webp",
    svg: "image/svg+xml",
    bmp: "image/bmp",
    ico: "image/x-icon",
  };
  return map[ext] ?? "";
}

/** Read a File as a base64 data: URL (local attachments, no upload server). */
function fileToDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

type Mode = "ask" | "plan" | "coding";

export default function AgentInput({ sessionId, folderPath, agentName = "the agent" }: AgentInputProps) {
  const [text, setText] = useState("");
  const [attachments, setAttachments] = useState<PendingAttachment[]>([]);
  const [mode, setMode] = useState<Mode>(
    () => (useAppStore.getState().sessions.find((s) => s.id === sessionId)?.mode as Mode) ?? "coding",
  );
  const [showPermissions, setShowPermissions] = useState(false);
  const [showSkills, setShowSkills] = useState(false);
  const [showSubagents, setShowSubagents] = useState(false);
  const [permissionLevel, setPermissionLevel] = useState<PermissionLevel>("ApproveDestructive");
  const [widthTier, setWidthTier] = useState<WidthTier>("wide");

  const [fileList, setFileList] = useState<string[]>([]);
  const [subagentList, setSubagentList] = useState<SubagentListEntry[]>([]);
  const [fileQuery, setFileQuery] = useState("");
  const [filePickerStartIdx, setFilePickerStartIdx] = useState(-1);
  const [showFilePicker, setShowFilePicker] = useState(false);
  const [selectedFileIdx, setSelectedFileIdx] = useState(0);
  const fileDropdownRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const SLASH_COMMANDS = useMemo(
    () => [
      { name: "clear", description: "Clear LLM context for this session" },
      { name: "compact", description: "Compact LLM context (summarize older messages)" },
      { name: "skills", description: "Manage skills available to the agent" },
      { name: "agents", description: "Manage subagents available to the agent" },
    ],
    [],
  );
  const [isCompacting, setIsCompacting] = useState(false);
  const [showCommandPicker, setShowCommandPicker] = useState(false);
  const [commandQuery, setCommandQuery] = useState("");
  const [selectedCommandIdx, setSelectedCommandIdx] = useState(0);
  const commandDropdownRef = useRef<HTMLDivElement>(null);

  const activeModel = useAppStore((s) => s.selection.active?.model ?? null);
  const tokenUsage = useAppStore((s) => s.tokenUsage[sessionId]);
  // Vision capability for the active model gates the image-attach affordances.
  const supportsImages = useAppStore((s) => s.activeCapability?.supportsImages ?? false);
  const [dragActive, setDragActive] = useState(false);

  // Load git-tracked files for @ picker.
  useEffect(() => {
    if (!folderPath) {
      setFileList([]);
      return;
    }
    invoke<string[]>("git_ls_files", { repoPath: folderPath })
      .then(setFileList)
      .catch(() => setFileList([]));
  }, [folderPath]);

  // Load enabled subagents.
  useEffect(() => {
    agentTauriService
      .listSubagents(folderPath ?? null)
      .then((list) => setSubagentList(list.filter((s) => s.enabled)))
      .catch(() => setSubagentList([]));
  }, [folderPath]);

  const filteredCommands = useMemo(() => {
    if (!showCommandPicker) return [];
    if (!commandQuery) return SLASH_COMMANDS;
    const q = commandQuery.toLowerCase();
    return SLASH_COMMANDS.filter((c) => c.name.toLowerCase().startsWith(q));
  }, [showCommandPicker, commandQuery, SLASH_COMMANDS]);

  const executeCommand = useCallback(
    async (commandName: string) => {
      if (isCompacting) return;
      setShowCommandPicker(false);
      setText("");

      if (commandName === "skills") {
        setShowSkills(true);
        return;
      }
      if (commandName === "agents") {
        setShowSubagents(true);
        return;
      }

      if (commandName === "clear") {
        try {
          await agentTauriService.clearContext(sessionId);
          useAppStore.getState().clearTokenUsage(sessionId);
          if (folderPath) useAppStore.getState().clearCompletedPlan(folderPath);
          themedMessage.success("Context cleared");
        } catch (err) {
          console.error("[AgentInput] Failed to clear context:", err);
          themedMessage.error("Failed to clear context");
        }
      } else if (commandName === "compact") {
        const toastKey = "compact-toast";
        setIsCompacting(true);
        themedMessage.loading("Compacting context...", toastKey);
        try {
          const result = await agentTauriService.compactContext(sessionId);
          if (result === "nothing_to_compact") {
            themedMessage.info("Not enough context to compact", toastKey);
          } else if (result === "truncated") {
            themedMessage.warning("Context truncated (summarization failed)", toastKey);
            useAppStore.getState().clearTokenUsage(sessionId);
          } else {
            themedMessage.success("Context compacted", toastKey);
            useAppStore.getState().clearTokenUsage(sessionId);
          }
        } catch (err) {
          console.error("[AgentInput] Failed to compact context:", err);
          themedMessage.error("Failed to compact context", toastKey);
        } finally {
          setIsCompacting(false);
        }
      }
    },
    [sessionId, folderPath, isCompacting],
  );

  type PickerItem =
    | { kind: "subagent"; name: string; description: string }
    | { kind: "file"; path: string };

  const pickerItems = useMemo<PickerItem[]>(() => {
    if (!showFilePicker) return [];
    const q = fileQuery.toLowerCase();
    const subs: PickerItem[] = [];
    for (const s of subagentList) {
      if (!q || s.name.toLowerCase().includes(q)) {
        subs.push({ kind: "subagent", name: s.name, description: s.description });
      }
    }
    const files: PickerItem[] = [];
    const fileBudget = Math.max(0, 15 - subs.length);
    for (const f of fileList) {
      if (files.length >= fileBudget) break;
      if (!q || f.toLowerCase().includes(q)) files.push({ kind: "file", path: f });
    }
    return [...subs, ...files];
  }, [fileList, subagentList, fileQuery, showFilePicker]);

  useEffect(() => {
    if (!showFilePicker) return;
    const handler = (e: MouseEvent) => {
      if (fileDropdownRef.current && !fileDropdownRef.current.contains(e.target as Node)) {
        setShowFilePicker(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [showFilePicker]);

  // Load persisted context usage on session/model change.
  useEffect(() => {
    useAppStore.getState().clearTokenUsage(sessionId);
    agentTauriService
      .getContextUsage(sessionId)
      .then((usage) => {
        if (usage) useAppStore.getState().setTokenUsage(sessionId, usage.total_tokens, usage.context_limit);
      })
      .catch(() => {});
  }, [sessionId, activeModel]);

  useEffect(() => {
    if (!showCommandPicker) return;
    const handler = (e: MouseEvent) => {
      if (commandDropdownRef.current && !commandDropdownRef.current.contains(e.target as Node)) {
        setShowCommandPicker(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [showCommandPicker]);

  const { send, cancel, isBusy, isSending } = useAgentSend({ sessionId });

  // Draft + mode keyed by session.
  useEffect(() => {
    const draft = loadDraft(sessionId);
    setText(draft?.text ?? "");
    const sessionMode = useAppStore.getState().sessions.find((s) => s.id === sessionId)?.mode as Mode | undefined;
    if (sessionMode) setMode(sessionMode);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  const handleModeChange = useCallback(
    (m: Mode) => {
      setMode(m);
      useAppStore.getState().setSessionMode(sessionId, m);
    },
    [sessionId],
  );

  const draftTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const handleTextChange = useCallback(
    (newText: string, cursorPos?: number) => {
      setText(newText);

      const slashMatch = newText.match(/^\/(\S*)$/);
      if (slashMatch) {
        setCommandQuery(slashMatch[1]);
        setShowCommandPicker(true);
        setSelectedCommandIdx(0);
      } else {
        setShowCommandPicker(false);
      }

      if (fileList.length > 0 || subagentList.length > 0) {
        const pos = cursorPos ?? newText.length;
        const before = newText.slice(0, pos);
        const atMatch = before.match(/@([^\s]*)$/);
        if (atMatch) {
          setFileQuery(atMatch[1]);
          setFilePickerStartIdx(pos - atMatch[0].length);
          setShowFilePicker(true);
          setSelectedFileIdx(0);
        } else {
          setShowFilePicker(false);
        }
      }

      if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
      draftTimerRef.current = setTimeout(() => saveDraft(sessionId, newText), 1000);
    },
    [sessionId, fileList.length, subagentList.length],
  );

  const isUploading = attachments.some((a) => a.uploading);

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    const uploadedAtts = attachments.filter((a) => a.uploaded).map((a) => a.uploaded!);
    if ((!trimmed && uploadedAtts.length === 0) || isSending || isUploading) return;
    setText("");
    setAttachments([]);
    if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
    clearDraft(sessionId);
    await send(trimmed, mode, uploadedAtts.length > 0 ? uploadedAtts : undefined);
  }, [text, attachments, isSending, isUploading, send, sessionId, mode]);

  const handleCancel = useCallback(async () => {
    await cancel();
  }, [cancel]);

  // Convert a local file to a data: URL attachment (no upload server).
  const trackAttachment = useCallback(async (file: File) => {
    const id = `att-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
    const mediaType = file.type || "application/octet-stream";
    setAttachments((prev) => [...prev, { id, file, file_name: file.name, uploading: true }]);
    try {
      const dataUrl = await fileToDataUrl(file);
      setAttachments((prev) =>
        prev.map((a) =>
          a.id === id
            ? {
                ...a,
                uploading: false,
                uploaded: { url: dataUrl, file_name: file.name, media_type: mediaType },
              }
            : a,
        ),
      );
    } catch {
      setAttachments((prev) => prev.map((a) => (a.id === id ? { ...a, uploading: false, error: "Failed to read file" } : a)));
    }
  }, []);

  const handleUploadClick = useCallback(async () => {
    const selected = await open({
      directory: false,
      multiple: true,
      title: "Select images",
      // Only images are supported (the backend forwards image/* attachments only).
      filters: [{ name: "Images", extensions: ["png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "ico"] }],
    });
    if (!selected) return;
    const paths = Array.isArray(selected) ? selected : [selected];
    for (const filePath of paths) {
      const fileName = filePath.split("/").pop() || filePath;
      // Guard against a non-image slipping through (e.g. the OS "All Files" override).
      if (!isImage(fileName)) continue;
      try {
        const bytes = await invoke<number[]>("read_file_bytes", { path: filePath }).catch(() => null);
        if (bytes) {
          // NOTE: File created from raw bytes has no MIME type, so file.type is "".
          // Infer it from the extension so the backend's image/* filter keeps it.
          const file = new File([new Uint8Array(bytes)], fileName, { type: mimeFromName(fileName) });
          await trackAttachment(file);
        }
      } catch {
        /* ignore */
      }
    }
  }, [trackAttachment]);

  const removeAttachment = (id: string) => setAttachments((prev) => prev.filter((a) => a.id !== id));

  const handlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      // Only intercept pasted images when the active model supports vision.
      if (!supportsImages) return;
      const items = e.clipboardData?.items;
      if (!items) return;
      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        if (item.type.startsWith("image/")) {
          e.preventDefault();
          const file = item.getAsFile();
          if (!file) continue;
          const ext = item.type.split("/")[1] || "png";
          trackAttachment(new File([file], `pasted-image-${Date.now()}.${ext}`, { type: file.type }));
          return;
        }
      }
    },
    [trackAttachment, supportsImages],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragActive(false);
      if (!supportsImages) return;
      const files = Array.from(e.dataTransfer?.files ?? []);
      for (const file of files) {
        if (file.type.startsWith("image/") || isImage(file.name)) trackAttachment(file);
      }
    },
    [trackAttachment, supportsImages],
  );

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      if (!supportsImages) return;
      e.preventDefault();
      setDragActive(true);
    },
    [supportsImages],
  );

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(false);
  }, []);

  const insertPickerItem = useCallback(
    (item: PickerItem) => {
      const before = text.slice(0, filePickerStartIdx);
      const after = text.slice(filePickerStartIdx + 1 + fileQuery.length);
      const insert = item.kind === "subagent" ? `@${item.name}` : item.path;
      setText(`${before}${insert} ${after}`);
      setShowFilePicker(false);
      requestAnimationFrame(() => textareaRef.current?.focus());
    },
    [text, filePickerStartIdx, fileQuery],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (showCommandPicker && filteredCommands.length > 0) {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedCommandIdx((p) => Math.min(p + 1, filteredCommands.length - 1));
          return;
        case "ArrowUp":
          e.preventDefault();
          setSelectedCommandIdx((p) => Math.max(p - 1, 0));
          return;
        case "Tab":
        case "Enter":
          e.preventDefault();
          executeCommand(filteredCommands[selectedCommandIdx].name);
          return;
        case "Escape":
          e.preventDefault();
          setShowCommandPicker(false);
          return;
      }
    }

    if (showFilePicker && pickerItems.length > 0) {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedFileIdx((p) => Math.min(p + 1, pickerItems.length - 1));
          return;
        case "ArrowUp":
          e.preventDefault();
          setSelectedFileIdx((p) => Math.max(p - 1, 0));
          return;
        case "Tab":
        case "Enter":
          e.preventDefault();
          insertPickerItem(pickerItems[selectedFileIdx]);
          return;
        case "Escape":
          e.preventDefault();
          setShowFilePicker(false);
          return;
      }
    } else if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const loadPermissionLevel = useCallback(async () => {
    try {
      const config = await agentTauriService.getPermissions(folderPath ?? undefined);
      setPermissionLevel(config.level as PermissionLevel);
    } catch {
      /* fallback */
    }
  }, [folderPath]);

  useEffect(() => {
    loadPermissionLevel();
  }, [loadPermissionLevel]);

  const filePickerDropdown =
    showFilePicker && pickerItems.length > 0 ? (
      <div
        ref={fileDropdownRef}
        className="absolute bottom-full left-0 right-0 mb-1 mx-3 max-h-[240px] overflow-y-auto bg-white dark:bg-dark-bg border border-gray-200 dark:border-dark-border rounded-lg shadow-lg z-50"
      >
        {pickerItems.map((item, i) => {
          const active = i === selectedFileIdx;
          const className = `w-full flex items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors ${
            active ? "bg-blue-50 dark:bg-blue-900/20" : "hover:bg-[var(--hover-bg)]"
          }`;
          if (item.kind === "subagent") {
            return (
              <button
                key={`sub:${item.name}`}
                type="button"
                className={className}
                onMouseDown={(e) => {
                  e.preventDefault();
                  insertPickerItem(item);
                }}
                onMouseEnter={() => setSelectedFileIdx(i)}
              >
                <Bot className="w-3.5 h-3.5 text-[#7B61FF] shrink-0" />
                <span className="truncate text-[var(--text-primary)]">@{item.name}</span>
                <span className="ml-auto text-[11px] text-[var(--text-secondary)] opacity-60 truncate max-w-[220px]">
                  {item.description}
                </span>
              </button>
            );
          }
          const fileName = item.path.split("/").pop() ?? item.path;
          const dirPath = item.path.includes("/") ? item.path.slice(0, item.path.lastIndexOf("/")) : "";
          return (
            <button
              key={`file:${item.path}`}
              type="button"
              className={className}
              onMouseDown={(e) => {
                e.preventDefault();
                insertPickerItem(item);
              }}
              onMouseEnter={() => setSelectedFileIdx(i)}
            >
              <FileIcon className="w-3.5 h-3.5 text-[var(--text-secondary)] shrink-0" />
              <span className="truncate text-[var(--text-primary)]">{fileName}</span>
              {dirPath && (
                <span className="ml-auto text-[11px] text-[var(--text-secondary)] opacity-60 truncate max-w-[200px]">
                  {dirPath}
                </span>
              )}
            </button>
          );
        })}
      </div>
    ) : null;

  const commandPickerDropdown =
    showCommandPicker && filteredCommands.length > 0 ? (
      <div
        ref={commandDropdownRef}
        className="absolute bottom-full left-0 right-0 mb-1 mx-3 overflow-hidden border rounded-lg shadow-lg z-50"
        style={{ backgroundColor: "var(--dropdown-bg)", boxShadow: "var(--dropdown-boxshadow)" }}
      >
        {filteredCommands.map((cmd, i) => (
          <button
            key={cmd.name}
            type="button"
            className={`dropdown_item ${i === selectedCommandIdx ? "bg-[var(--white-opacity-8)]" : ""}`}
            onMouseDown={(e) => {
              e.preventDefault();
              executeCommand(cmd.name);
            }}
            onMouseEnter={() => setSelectedCommandIdx(i)}
          >
            <span className="font-mono text-xs text-[var(--text-color)]">/{cmd.name}</span>
            <span className="text-xs text-[var(--white-opacity-60)]">{cmd.description}</span>
          </button>
        ))}
      </div>
    ) : null;

  const permissionLabel = PERMISSION_LABELS[permissionLevel] ?? "Balanced";
  const isNarrow = widthTier === "narrow";
  const isMedium = widthTier === "medium";

  const contextIndicator = (() => {
    if (!tokenUsage) return null;
    const { totalTokens, contextLimit } = tokenUsage;
    if (!totalTokens) return null;
    const fmt = (n: number) => (n >= 1_000_000 ? `${(n / 1_000_000).toFixed(1)}M` : n >= 1000 ? `${(n / 1000).toFixed(0)}k` : String(n));
    // Unknown context window → dashed ring + raw token count (no gauge, no %).
    if (!contextLimit || contextLimit === 0) {
      return (
        <Tooltip title={`${fmt(totalTokens)} tokens used · context window unknown for this model`}>
          <div className="flex items-center gap-1 shrink-0 mr-1 cursor-default">
            <CircleDashed className="w-3.5 h-3.5 text-[var(--text-secondary)]" />
            <span className="text-[10px] text-[var(--text-secondary)] font-mono">{fmt(totalTokens)}</span>
          </div>
        </Tooltip>
      );
    }
    const rawPercent = (totalTokens / contextLimit) * 100;
    const usagePercent = rawPercent > 0 && rawPercent < 1 ? 1 : Math.round(rawPercent);
    const displayPercent = rawPercent > 0 && rawPercent < 10 ? rawPercent.toFixed(1) : String(usagePercent);
    const usageColor = usagePercent > 80 ? "#f5222d" : usagePercent > 60 ? "#faad14" : "#52c41a";
    return (
      <Tooltip title={`${fmt(totalTokens)} / ${fmt(contextLimit)} tokens (${displayPercent}%)`}>
        <div className="flex items-center gap-1 shrink-0 mr-1 cursor-default">
          <Progress type="circle" percent={usagePercent} size={14} strokeWidth={12} strokeColor={usageColor} showInfo={false} />
          <span className="text-[10px] text-[var(--text-secondary)] font-mono">{displayPercent}%</span>
        </div>
      </Tooltip>
    );
  })();

  // Image attach is the only attachment the backend forwards, so the upload
  // affordance is shown only when the active model supports vision.
  const uploadButton = supportsImages ? (
    <button
      onClick={handleUploadClick}
      className="p-1.5 rounded-md text-gray-500 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-[var(--hover-bg)]"
      title="Attach image"
    >
      <Plus className="w-4 h-4" />
    </button>
  ) : null;

  const permissionChipFull = (
    <ActionChip
      label={permissionLabel}
      prefix={<Code className="w-4 h-4 text-gray-500" />}
      onClick={() => setShowPermissions(true)}
    />
  );

  const permissionChipIcon = (
    <button
      onClick={() => setShowPermissions(true)}
      title={permissionLabel}
      className="p-1.5 rounded-md text-gray-500 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-[var(--hover-bg)]"
    >
      <Code className="w-4 h-4" />
    </button>
  );

  const modeSegmented = (
    <Segmented
      size="small"
      value={mode === "plan" ? "Plan" : mode === "coding" ? "Code" : "Ask"}
      options={[
        { label: <span className="flex items-center gap-1"><MessageCircle size={12} />{!isNarrow && <span>Ask</span>}</span>, value: "Ask" },
        { label: <span className="flex items-center gap-1"><Map size={12} />{!isNarrow && <span>Plan</span>}</span>, value: "Plan" },
        { label: <span className="flex items-center gap-1"><Code size={12} />{!isNarrow && <span>Code</span>}</span>, value: "Code" },
      ]}
      onChange={(val) => handleModeChange(val === "Plan" ? "plan" : val === "Code" ? "coding" : "ask")}
      style={{ fontSize: 12, backgroundColor: "var(--white-opacity-10)" }}
    />
  );

  // Narrow tier: model picker, permission, and context usage collapse into a "…" menu.
  const overflowMenu = (
    <Popover
      trigger="click"
      placement="topRight"
      content={
        <div className="flex flex-col gap-2 min-w-[180px]">
          <ModelPicker />
          {permissionChipFull}
          {contextIndicator ?? (
            <span className="text-xs text-[var(--text-secondary)]">No context usage yet</span>
          )}
        </div>
      }
    >
      <button
        className="p-1.5 rounded-md text-gray-500 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-[var(--hover-bg)]"
        title="More controls"
      >
        <MoreHorizontal className="w-4 h-4" />
      </button>
    </Popover>
  );

  const toolbarLeftNode = isNarrow ? (
    <>
      {uploadButton}
      {overflowMenu}
    </>
  ) : (
    <>
      {uploadButton}
      <ModelPicker />
      {isMedium ? permissionChipIcon : permissionChipFull}
    </>
  );

  const toolbarRightNode = (
    <div className="flex items-center">
      {!isNarrow && contextIndicator}
      {modeSegmented}
    </div>
  );

  const attachmentPreviews =
    attachments.length > 0 ? (
      <div className="flex flex-wrap gap-2 px-4 pt-3">
        {attachments.map((att) => (
          <div
            key={att.id}
            className="relative flex items-center gap-2 bg-white dark:bg-dark-bg border border-gray-200 dark:border-dark-border rounded-lg px-3 py-2 text-sm"
          >
            {isImage(att.file_name) && att.file.size > 0 ? (
              <img src={URL.createObjectURL(att.file)} alt={att.file_name} className="w-10 h-10 object-cover rounded" />
            ) : (
              <FileText className="w-5 h-5 text-gray-400 shrink-0" />
            )}
            <div className="flex flex-col min-w-0">
              <span className="text-gray-700 dark:text-gray-300 truncate max-w-[150px]">{att.file_name}</span>
              {att.uploading && <span className="text-xs text-blue-500">Reading...</span>}
              {att.error && <span className="text-xs text-red-500">{att.error}</span>}
            </div>
            <button
              onClick={() => removeAttachment(att.id)}
              className="absolute -top-1.5 -right-1.5 bg-gray-500 text-white rounded-full p-0.5 hover:bg-gray-700"
            >
              <X className="w-3 h-3" />
            </button>
          </div>
        ))}
      </div>
    ) : null;

  return (
    <>
      <div
        className="relative"
        onDrop={supportsImages ? handleDrop : undefined}
        onDragOver={supportsImages ? handleDragOver : undefined}
        onDragLeave={supportsImages ? handleDragLeave : undefined}
      >
        {dragActive && (
          <div className="absolute inset-0 z-20 m-5 rounded-xl border-2 border-dashed border-blue-400 bg-blue-50/70 dark:bg-blue-500/10 flex items-center justify-center pointer-events-none">
            <span className="text-sm text-blue-600 dark:text-blue-300">Drop image to attach</span>
          </div>
        )}
        <InputShell
          value={text}
          onChange={(e) => handleTextChange(e.target.value, e.target.selectionStart ?? undefined)}
          textareaRef={textareaRef}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          placeholder={`Ask ${agentName} to do something...`}
          onSend={isBusy ? handleCancel : handleSend}
          sendDisabled={
            isBusy ? false : (!text.trim() && attachments.every((a) => !a.uploaded)) || isSending || isUploading || isCompacting
          }
          isStop={isBusy}
          onWidthChange={setWidthTier}
          innerContent={
            <>
              {commandPickerDropdown}
              {filePickerDropdown}
              {attachmentPreviews}
            </>
          }
          toolbarRight={toolbarRightNode}
          toolbarLeft={toolbarLeftNode}
        />
      </div>
      <PermissionSettingsModal
        isOpen={showPermissions}
        onClose={() => {
          setShowPermissions(false);
          loadPermissionLevel();
        }}
        projectPath={folderPath}
      />
      <SkillsDialog isOpen={showSkills} onClose={() => setShowSkills(false)} workingDir={folderPath} />
      <SubagentsDialog isOpen={showSubagents} onClose={() => setShowSubagents(false)} workingDir={folderPath} />
    </>
  );
}
