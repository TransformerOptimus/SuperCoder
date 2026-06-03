import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Skeleton } from "antd";
import { Maximize2, Code } from "lucide-react";
import { useAppStore } from "@/store";
import AgentMessageBubble from "../AgentMessageBubble/AgentMessageBubble";
import ArtifactRenderer from "../artifacts/ArtifactRenderer";
import AgentInput from "../AgentInput/AgentInput";
import ApprovalBanner from "./ApprovalBanner";
import QuestionBanner from "./QuestionBanner";
import TodoProgress from "./TodoProgress";
import PlanBanner from "./PlanBanner";
import RewindEditor from "./RewindEditor";
import ThreadPanelHeader from "./components/ThreadPanelHeader/ThreadPanelHeader";
import { buildAgentMessage } from "@/utils/agentMessageAdapter";
import { agentTauriService } from "@/services/agentTauriService";
import { createInitialStreamingState } from "@/store/agentSlice";
import type { AgentMessage } from "@/types/agent";
import type { TodoItem } from "@/types/agentContract";
import styles from "./AgentThreadPanel.module.css";

const EMPTY_TODOS: TodoItem[] = [];

/** Pull the media type out of a `data:<media>;base64,…` URL (defaults to png). */
function dataUrlMediaType(url: string): string {
  return /^data:([^;,]+)/.exec(url)?.[1] || "image/png";
}

export default function AgentThreadPanel() {
  const activeAgentThreadId = useAppStore((s) => s.activeAgentThreadId);
  const agentThreads = useAppStore((s) => s.agentThreads);
  const expandArtifactDiff = useAppStore((s) => s.expandArtifactDiff);
  const pendingApprovals = useAppStore((s) => s.pendingApprovals);
  const activeSessionIds = useAppStore((s) => s.activeSessionIds);
  const removePendingApproval = useAppStore((s) => s.removePendingApproval);
  const pendingQuestion = useAppStore((s) => (activeAgentThreadId ? s.pendingQuestions[activeAgentThreadId] : null));
  const clearPendingQuestion = useAppStore((s) => s.clearPendingQuestion);
  const todos = useAppStore((s) => (activeAgentThreadId ? s.agentTodos[activeAgentThreadId] ?? EMPTY_TODOS : EMPTY_TODOS));

  const streaming = useAppStore((s) => (activeAgentThreadId ? s.agentStreaming[activeAgentThreadId] : null));
  const isLoadingThread = useAppStore((s) => (activeAgentThreadId ? !!s.agentThreadLoading[activeAgentThreadId] : false));

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const thread = activeAgentThreadId ? agentThreads[activeAgentThreadId] : null;

  const [submittingId, setSubmittingId] = useState<string | null>(null);
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null);
  const [editingText, setEditingText] = useState("");
  // Image data-URLs kept for the message being edited (user can remove them).
  const [editingImages, setEditingImages] = useState<string[]>([]);
  const isRewindingRef = useRef(false);

  const rewindThread = useAppStore((s) => s.rewindThread);
  const addMessageToThread = useAppStore((s) => s.addMessageToThread);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const addCheckpoint = useAppStore((s) => s.addCheckpoint);

  const handleStartEdit = useCallback((msgId: string, text: string, images?: string[]) => {
    setEditingMessageId(msgId);
    setEditingText(text);
    setEditingImages(images ?? []);
  }, []);

  const handleCancelEdit = useCallback(() => {
    setEditingMessageId(null);
    setEditingText("");
    setEditingImages([]);
  }, []);

  const handleRemoveEditingImage = useCallback((idx: number) => {
    setEditingImages((prev) => prev.filter((_, i) => i !== idx));
  }, []);

  const handleRewind = useCallback(
    async (restoreCode: boolean) => {
      if (!editingMessageId || !activeAgentThreadId || !thread || isRewindingRef.current) return;
      const targetIdx = thread.messages.findIndex((m) => m.id === editingMessageId);
      if (targetIdx < 0) return;
      const newText = editingText.trim();
      const keptImages = editingImages;
      // Allow resending with only images (no text), but not a fully empty message.
      if (!newText && keptImages.length === 0) return;

      // Backend rewind needs the SQLite row id (numeric). Only available after `done`.
      const sqliteId = Number(editingMessageId);
      if (!Number.isFinite(sqliteId)) {
        console.warn("[AgentThreadPanel] message not yet persisted — cannot rewind");
        return;
      }

      const attachments = keptImages.map((url) => ({
        url,
        file_name: "image",
        media_type: dataUrlMediaType(url),
      }));

      isRewindingRef.current = true;
      rewindThread(activeAgentThreadId, targetIdx);
      addMessageToThread(
        activeAgentThreadId,
        buildAgentMessage(
          `rewind-user-${Date.now()}`,
          newText,
          "user",
          activeAgentThreadId,
          "",
          undefined,
          undefined,
          keptImages.length > 0 ? keptImages : undefined,
        ),
      );
      setEditingMessageId(null);
      setEditingText("");
      setEditingImages([]);
      useAppStore.getState().setAgentStreaming(activeAgentThreadId, createInitialStreamingState());

      try {
        const { session_id } = await agentTauriService.rewindToMessage(
          activeAgentThreadId,
          sqliteId,
          restoreCode,
          newText,
          attachments.length > 0 ? attachments : undefined,
        );
        setActiveSession(activeAgentThreadId, session_id);
      } catch (err) {
        console.error("[AgentThreadPanel] Rewind failed:", err);
      } finally {
        isRewindingRef.current = false;
      }
    },
    [editingMessageId, editingText, editingImages, activeAgentThreadId, thread, rewindThread, addMessageToThread, setActiveSession],
  );

  const handleApprovalResponse = useCallback(
    async (toolCallId: string, approved: boolean) => {
      if (!thread || submittingId) return;
      setSubmittingId(toolCallId);
      try {
        const sessionId = activeSessionIds[thread.id] ?? thread.id;
        await agentTauriService.approveToolCall(sessionId, toolCallId, approved);
        removePendingApproval(toolCallId);
      } catch (err) {
        console.error("[AgentThreadPanel] approveToolCall failed:", err);
      } finally {
        setSubmittingId(null);
      }
    },
    [thread, activeSessionIds, removePendingApproval, submittingId],
  );

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [thread?.messages.length, streaming?.textBuffer, thread?.total_additions, thread?.total_deletions]);

  // Load checkpoints when the thread opens.
  useEffect(() => {
    if (!activeAgentThreadId) return;
    let cancelled = false;
    agentTauriService
      .listCheckpoints(activeAgentThreadId)
      .then((checkpoints) => {
        if (!cancelled) checkpoints.forEach((cp) => addCheckpoint(activeAgentThreadId!, cp));
      })
      .catch((err) => console.warn("[AgentThreadPanel] Failed to load checkpoints:", err));
    return () => {
      cancelled = true;
    };
  }, [activeAgentThreadId, addCheckpoint]);

  // Load persisted token usage on open.
  useEffect(() => {
    if (!activeAgentThreadId) return;
    agentTauriService
      .getContextUsage(activeAgentThreadId)
      .then((usage) => {
        if (usage) useAppStore.getState().setTokenUsage(activeAgentThreadId!, usage.total_tokens, usage.context_limit);
      })
      .catch(() => {});
  }, [activeAgentThreadId]);

  // Keep the header / sticky-card diff totals in sync with the real working-tree
  // diff (the same source the Code Changes panel uses). Refetch on open, after
  // each completed turn (checkpoint added), and when streaming ends.
  const checkpointCount = thread?.checkpoints.length ?? 0;
  const isStreaming = !!streaming?.isStreaming;
  useEffect(() => {
    if (!activeAgentThreadId) return;
    let cancelled = false;
    agentTauriService
      .getFullDiff(activeAgentThreadId)
      .then((d) => {
        if (!cancelled) useAppStore.getState().setThreadDiffStats(activeAgentThreadId!, d.insertions, d.deletions, d.files_changed);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [activeAgentThreadId, checkpointCount, isStreaming]);

  if (!thread) return null;

  const handleExpandArtifact = (artifactId: string) => expandArtifactDiff(thread.id, artifactId);

  const threadApprovals = Object.values(pendingApprovals).filter((a) => a.threadId === thread.id);

  const streamingMessage: AgentMessage | null = streaming?.textBuffer
    ? {
        id: `${thread.id}-streaming`,
        thread_id: thread.id,
        agent_id: "",
        role: "agent",
        text: streaming.textBuffer,
        artifacts: [],
        created_at: new Date().toISOString(),
      }
    : null;

  return (
    <div className={styles.panel}>
      <ThreadPanelHeader
        taskSummary={thread.task_summary}
        folderPath={thread.folder_path}
        branch={thread.branch}
        filesChanged={thread.files_changed ?? 0}
        totalAdditions={thread.total_additions}
        totalDeletions={thread.total_deletions}
        onExpandDiff={
          thread.total_additions > 0 || thread.total_deletions > 0
            ? () => expandArtifactDiff(thread.id, `diff-full-${thread.id}`)
            : undefined
        }
      />
      {thread.sourcePlanText && <PlanBanner planText={thread.sourcePlanText} />}
      {isLoadingThread && thread.messages.length === 0 && (
        <div className="flex-1 flex flex-col justify-end gap-5 px-5 py-4">
          {[70, 45, 80, 35, 60].map((width, i) => (
            <div key={i} className="flex items-start gap-3">
              <Skeleton.Avatar active size={36} />
              <div className="flex-1" style={{ maxWidth: `${width}%` }}>
                <Skeleton active title={{ width: "40%" }} paragraph={{ rows: i % 2 === 0 ? 2 : 1, width: i % 2 === 0 ? ["100%", "60%"] : ["80%"] }} />
              </div>
            </div>
          ))}
        </div>
      )}
      <div className={styles.messages}>
        <div className={styles.inner}>
        {thread.messages.map((msg) => {
          const isEditing = msg.id === editingMessageId;
          const isUserMsg = msg.role === "user";
          const canRewind = isUserMsg && !streaming?.isStreaming && !isRewindingRef.current && Number.isFinite(Number(msg.id));

          return (
            <div key={msg.id} className={styles.message_group}>
              {isEditing ? (
                <RewindEditor
                  text={editingText}
                  images={editingImages}
                  onRemoveImage={handleRemoveEditingImage}
                  onChange={setEditingText}
                  onCancel={handleCancelEdit}
                  onRewind={handleRewind}
                  isCodingSession={!!thread.is_coding_session}
                  isRewinding={!!streaming?.isStreaming}
                />
              ) : (
                <AgentMessageBubble
                  message={msg}
                  onRewindAgent={
                    canRewind
                      ? {
                          onEditAndResend: () => handleStartEdit(msg.id, msg.text, msg.images),
                          isCodingSession: !!thread.is_coding_session,
                          hasCheckpoints: (thread.checkpoints?.length ?? 0) > 1,
                          isStreaming: !!streaming?.isStreaming,
                          isRewinding: isRewindingRef.current,
                        }
                      : undefined
                  }
                />
              )}
              {!isEditing && msg.artifacts.length > 0 && (
                <div className={styles.artifacts}>
                  {msg.artifacts.map((artifact) => (
                    <ArtifactRenderer key={artifact.id} artifact={artifact} inline onExpand={handleExpandArtifact} />
                  ))}
                </div>
              )}
            </div>
          );
        })}

        {streamingMessage && (
          <div className={styles.message_group}>
            <AgentMessageBubble message={streamingMessage} />
          </div>
        )}

        {streaming?.error && (
          <div className="mx-1 my-2">
            <Alert type={streaming.error.includes("interrupted") ? "warning" : "error"} showIcon message={streaming.error} />
          </div>
        )}

        {streaming?.isStreaming && !streaming.textBuffer && (() => {
          const runningSubagent = streaming.toolCalls.find(
            (tc) => tc.status === "running" && tc.toolName.toLowerCase().startsWith("subagent:"),
          );
          const thinkingText = runningSubagent
            ? `__thinking_subagent:${runningSubagent.toolName.slice(runningSubagent.toolName.indexOf(":") + 1).trim()}__`
            : "__thinking__";
          return (
            <div className={styles.message_group}>
              <AgentMessageBubble
                message={{
                  id: `${thread.id}-thinking`,
                  thread_id: thread.id,
                  agent_id: "",
                  role: "agent",
                  text: thinkingText,
                  artifacts: [],
                  created_at: new Date().toISOString(),
                }}
              />
            </div>
          );
        })()}

        {threadApprovals.map((approval) => (
          <ApprovalBanner
            key={approval.toolCallId}
            approval={approval}
            onApprove={() => handleApprovalResponse(approval.toolCallId, true)}
            onDeny={() => handleApprovalResponse(approval.toolCallId, false)}
            disabled={submittingId === approval.toolCallId}
          />
        ))}

        {pendingQuestion && activeAgentThreadId && (
          <QuestionBanner
            question={pendingQuestion.question}
            options={pendingQuestion.options}
            onAnswer={(answer) => {
              clearPendingQuestion(activeAgentThreadId);
              agentTauriService.sendMessage(activeAgentThreadId, answer);
            }}
            onSkip={() => {
              clearPendingQuestion(activeAgentThreadId);
              agentTauriService.sendMessage(activeAgentThreadId, "Skip — proceed with your best judgment.");
            }}
          />
        )}

        {todos.length > 0 && <TodoProgress todos={todos} />}

        <div ref={messagesEndRef} />
        </div>
      </div>

      {(thread.total_additions > 0 || thread.total_deletions > 0) && (() => {
        const fileCount = thread.files_changed ?? new Set(thread.checkpoints?.flatMap((cp) => cp.files ?? []) ?? []).size;
        return (
          <div
            className={`${styles.diff_card} mb-1 flex items-center gap-2 px-3 py-1.5 border border-[var(--border-color-8)] rounded-lg bg-[var(--bg-secondary)] cursor-pointer transition-colors hover:border-[var(--text-secondary)] flex-shrink-0`}
            onClick={() => expandArtifactDiff(thread.id, `diff-full-${thread.id}`)}
          >
            <Code className="w-3.5 h-3.5 text-gray-500 shrink-0" />
            <span className="text-[12px] text-[var(--text-primary)] flex-1">Code Changes</span>
            <span className="text-[11px] text-[var(--text-secondary)]">{fileCount} file{fileCount !== 1 ? "s" : ""}</span>
            <span className="text-[11px] font-mono text-[var(--diff-add-color)]">+{thread.total_additions}</span>
            <span className="text-[11px] font-mono text-[var(--diff-del-color)]">-{thread.total_deletions}</span>
            <Maximize2 className="w-3.5 h-3.5 text-gray-500 opacity-50 hover:opacity-100 shrink-0" />
          </div>
        );
      })()}
      <div className={styles.input_area}>
        <AgentInput sessionId={thread.id} folderPath={thread.folder_path} agentName="the agent" />
      </div>
    </div>
  );
}
