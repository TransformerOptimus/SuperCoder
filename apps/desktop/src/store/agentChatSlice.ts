import type { StateCreator } from 'zustand';
import type {
  AgentThread,
  AgentMessage,
  AgentViewMode,
  Artifact,
  ArtifactFileDecision,
  CheckpointSummary,
  SessionRow,
} from '../types/agent';
import type { AppStore } from './types';

export interface AgentChatSlice {
  /** All sessions for the sidebar (from the `sessions` table), most recent first. */
  sessions: SessionRow[];
  sessionsLoading: boolean;
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  setSessions: (sessions: SessionRow[]) => void;
  upsertSession: (session: SessionRow) => void;
  removeSession: (sessionId: string) => void;
  setSessionMode: (sessionId: string, mode: string) => void;
  setSessionTitle: (sessionId: string, title: string) => void;
  /** Re-pin an open session's model (picker switch); refreshes capability + context bar. */
  setSessionModel: (sessionId: string, providerId: string, model: string) => Promise<void>;
  /** Sync the in-memory active model + capability to a session's pinned model on open. */
  syncPickerToSession: (sessionId: string) => void;
  loadSessions: () => Promise<void>;

  agentThreads: Record<string, AgentThread>;
  /** Per-thread loading flag — true while initial messages/diff/plan are being fetched */
  agentThreadLoading: Record<string, boolean>;
  activeAgentThreadId: string | null;
  agentViewMode: AgentViewMode;
  expandedArtifactId: string | null;
  fileDecisions: Record<string, ArtifactFileDecision[]>;
  showFileExplorer: boolean;
  showTerminal: boolean;

  upsertAgentThread: (thread: AgentThread) => void;
  removeAgentThread: (threadId: string) => void;
  setAgentThreadLoading: (threadId: string, loading: boolean) => void;

  openAgentThread: (threadId: string) => void;
  closeAgentThread: () => void;
  expandArtifactDiff: (threadId: string, artifactId: string) => void;
  closeDiffReview: () => void;
  toggleFileExplorer: () => void;
  toggleTerminal: () => void;

  setFileDecision: (artifactId: string, filePath: string, decision: 'accepted' | 'rejected') => void;

  addMessageToThread: (threadId: string, message: AgentMessage) => void;
  addArtifactsToMessage: (threadId: string, messageId: string, artifacts: Artifact[]) => void;
  updateThreadStatus: (threadId: string, status: AgentThread['status']) => void;
  markThreadAsCodingSession: (threadId: string) => void;

  /** Set the thread's cumulative diff totals directly (from the working-tree diff). */
  setThreadDiffStats: (threadId: string, additions: number, deletions: number, filesChanged: number) => void;
  addCheckpoint: (threadId: string, checkpoint: CheckpointSummary) => void;
  setCheckpoints: (threadId: string, checkpoints: CheckpointSummary[]) => void;
  truncateCheckpoints: (threadId: string, afterTurn: number) => void;
  setSelectedDiffTurn: (threadId: string, turn: number | null) => void;
  replaceThreadMessages: (threadId: string, messages: AgentMessage[]) => void;
  rewindThread: (threadId: string, targetMessageIndex: number) => void;
  refreshMessageIds: (threadId: string, sqliteMessages: { id: string; role: string; text: string }[]) => void;
}

export const createAgentChatSlice: StateCreator<AppStore, [], [], AgentChatSlice> = (set, get) => ({
  sessions: [],
  sessionsLoading: false,
  sidebarCollapsed: false,

  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),

  setSessions: (sessions) => set({ sessions }),

  upsertSession: (session) =>
    set((s) => {
      const rest = s.sessions.filter((x) => x.id !== session.id);
      return { sessions: [session, ...rest] };
    }),

  removeSession: (sessionId) =>
    set((s) => ({ sessions: s.sessions.filter((x) => x.id !== sessionId) })),

  setSessionMode: (sessionId, mode) =>
    set((s) => ({
      sessions: s.sessions.map((x) => (x.id === sessionId ? { ...x, mode } : x)),
      agentThreads: s.agentThreads[sessionId]
        ? { ...s.agentThreads, [sessionId]: { ...s.agentThreads[sessionId], is_coding_session: mode === "coding" } }
        : s.agentThreads,
    })),

  setSessionTitle: (sessionId, title) =>
    set((s) => ({
      sessions: s.sessions.map((x) => (x.id === sessionId ? { ...x, title } : x)),
      agentThreads: s.agentThreads[sessionId]
        ? { ...s.agentThreads, [sessionId]: { ...s.agentThreads[sessionId], task_summary: title } }
        : s.agentThreads,
    })),

  setSessionModel: async (sessionId, providerId, model) => {
    const { agentTauriService } = await import('../services/agentTauriService');
    // Optimistically update the session row + the picker's in-memory active model.
    set((s) => ({
      sessions: s.sessions.map((x) => (x.id === sessionId ? { ...x, providerId, model } : x)),
      selection: { ...s.selection, active: { providerId, model } },
    }));
    try {
      await agentTauriService.setSessionModel(sessionId, providerId, model);
    } catch (e) {
      console.error('[agentChatSlice] Failed to set session model:', e);
    }
    // Re-resolve vision gating + the context bar for the newly pinned model.
    await get().refreshActiveCapability();
    try {
      const usage = await agentTauriService.getContextUsage(sessionId);
      if (usage) get().setTokenUsage(sessionId, usage.total_tokens, usage.context_limit);
    } catch {
      /* ignore */
    }
  },

  syncPickerToSession: (sessionId) => {
    const sess = get().sessions.find((s) => s.id === sessionId);
    if (sess?.providerId && sess?.model) {
      set((s) => ({ selection: { ...s.selection, active: { providerId: sess.providerId!, model: sess.model! } } }));
      void get().refreshActiveCapability();
    }
  },

  loadSessions: async () => {
    set({ sessionsLoading: true });
    try {
      const { agentTauriService } = await import('../services/agentTauriService');
      const sessions = await agentTauriService.listSessions();
      set({ sessions });
    } catch (e) {
      console.error('[agentChatSlice] Failed to load sessions:', e);
    } finally {
      set({ sessionsLoading: false });
    }
  },

  agentThreads: {},
  agentThreadLoading: {},
  activeAgentThreadId: null,
  agentViewMode: 'chat',
  expandedArtifactId: null,
  fileDecisions: {},
  showFileExplorer: false,
  showTerminal: false,

  upsertAgentThread: (thread) =>
    set((s) => ({
      agentThreads: { ...s.agentThreads, [thread.id]: thread },
    })),

  removeAgentThread: (threadId) =>
    set((s) => {
      const { [threadId]: _, ...rest } = s.agentThreads;
      return { agentThreads: rest };
    }),

  setAgentThreadLoading: (threadId, loading) =>
    set((s) => {
      if (loading) {
        return { agentThreadLoading: { ...s.agentThreadLoading, [threadId]: true } };
      }
      const { [threadId]: _, ...rest } = s.agentThreadLoading;
      return { agentThreadLoading: rest };
    }),

  openAgentThread: (threadId) =>
    set({ activeAgentThreadId: threadId }),

  closeAgentThread: () => {
    const threadId = get().activeAgentThreadId;
    if (threadId) {
      get().clearAgentStreaming(threadId);
    }
    set({ agentViewMode: 'chat', activeAgentThreadId: null, expandedArtifactId: null });
  },

  expandArtifactDiff: (threadId, artifactId) => {
    set({
      agentViewMode: 'diff_review' as const,
      activeAgentThreadId: threadId,
      expandedArtifactId: artifactId,
    });
  },

  closeDiffReview: () =>
    set({ agentViewMode: 'thread', expandedArtifactId: null }),

  toggleFileExplorer: () =>
    set((s) => ({ showFileExplorer: !s.showFileExplorer })),

  toggleTerminal: () =>
    set((s) => ({ showTerminal: !s.showTerminal })),

  setFileDecision: (artifactId, filePath, decision) =>
    set((s) => {
      const current = s.fileDecisions[artifactId] || [];
      const updated = current.map((d) =>
        d.filePath === filePath ? { ...d, decision } : d,
      );
      return { fileDecisions: { ...s.fileDecisions, [artifactId]: updated } };
    }),

  addMessageToThread: (threadId, message) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) {
        console.warn('[addMessageToThread] Thread not found:', threadId, 'message dropped:', message.id);
        return s;
      }
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: {
            ...thread,
            messages: [...thread.messages, message],
            updated_at: new Date().toISOString(),
          },
        },
      };
    }),

  addArtifactsToMessage: (threadId, messageId, artifacts) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      const msgIndex = thread.messages.findIndex((m) => m.id === messageId);
      if (msgIndex === -1) return s;
      const updatedMessages = [...thread.messages];
      const existingIds = new Set(updatedMessages[msgIndex].artifacts.map((a) => a.id));
      const newArtifacts = artifacts.filter((a) => !existingIds.has(a.id));
      if (newArtifacts.length === 0) return s;
      updatedMessages[msgIndex] = {
        ...updatedMessages[msgIndex],
        artifacts: [...updatedMessages[msgIndex].artifacts, ...newArtifacts],
      };
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, messages: updatedMessages, updated_at: new Date().toISOString() },
        },
      };
    }),

  updateThreadStatus: (threadId, status) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, status },
        },
      };
    }),

  markThreadAsCodingSession: (threadId) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, is_coding_session: true },
        },
      };
    }),

  setThreadDiffStats: (threadId, additions, deletions, filesChanged) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      if (
        thread.total_additions === additions &&
        thread.total_deletions === deletions &&
        thread.files_changed === filesChanged
      )
        return s;
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, total_additions: additions, total_deletions: deletions, files_changed: filesChanged },
        },
      };
    }),

  addCheckpoint: (threadId, checkpoint) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      const existing = thread.checkpoints.findIndex(
        (cp) => cp.turn_count === checkpoint.turn_count,
      );
      const updated = [...thread.checkpoints];
      if (existing >= 0) {
        updated[existing] = checkpoint;
      } else {
        updated.push(checkpoint);
        updated.sort((a, b) => a.turn_count - b.turn_count);
      }
      // Recompute thread-level diff totals from all checkpoints (single source of truth)
      // so the live "Code Changes" sticky card updates without waiting for a chat switch.
      const total_additions = updated.reduce((sum, cp) => sum + (cp.additions ?? 0), 0);
      const total_deletions = updated.reduce((sum, cp) => sum + (cp.deletions ?? 0), 0);
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, checkpoints: updated, total_additions, total_deletions },
        },
      };
    }),

  setCheckpoints: (threadId, checkpoints) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      const sorted = [...checkpoints].sort((a, b) => a.turn_count - b.turn_count);
      const total_additions = sorted.reduce((sum, cp) => sum + (cp.additions ?? 0), 0);
      const total_deletions = sorted.reduce((sum, cp) => sum + (cp.deletions ?? 0), 0);
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, checkpoints: sorted, total_additions, total_deletions },
        },
      };
    }),

  truncateCheckpoints: (threadId, afterTurn) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      const filtered = thread.checkpoints.filter((cp) => cp.turn_count <= afterTurn);
      const total_additions = filtered.reduce((sum, cp) => sum + (cp.additions ?? 0), 0);
      const total_deletions = filtered.reduce((sum, cp) => sum + (cp.deletions ?? 0), 0);
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: {
            ...thread,
            checkpoints: filtered,
            total_additions,
            total_deletions,
            selectedDiffTurn: null,
          },
        },
      };
    }),

  setSelectedDiffTurn: (threadId, turn) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, selectedDiffTurn: turn },
        },
      };
    }),

  replaceThreadMessages: (threadId, messages) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, messages },
        },
      };
    }),

  refreshMessageIds: (threadId, sqliteMessages) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      // Find the last client-generated user message in the store (there's at most one —
      // the message we just sent via rewind or follow-up).
      const lastClientMsg = [...thread.messages].reverse().find(
        (m) => m.role === "user" && (m.id.startsWith("rewind-user-") || m.id.startsWith("agent-user-")),
      );
      if (!lastClientMsg) return s;
      // Find the last user message from SQLite — this is the same message, now persisted.
      const lastSqliteUser = [...sqliteMessages].reverse().find((m) => m.role === "user");
      if (!lastSqliteUser || lastSqliteUser.id === lastClientMsg.id) return s;
      // Update just that one message's ID
      const updated = thread.messages.map((msg) =>
        msg.id === lastClientMsg.id ? { ...msg, id: lastSqliteUser.id } : msg,
      );
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: { ...thread, messages: updated },
        },
      };
    }),

  rewindThread: (threadId, targetMessageIndex) =>
    set((s) => {
      const thread = s.agentThreads[threadId];
      if (!thread) return s;
      // Strip artifacts from remaining messages — old per-turn diffs are stale
      // after rewind. The checkpoint reconstruction effect will re-attach fresh ones.
      const kept = thread.messages.slice(0, targetMessageIndex).map((m) =>
        m.artifacts.length > 0 ? { ...m, artifacts: [] } : m
      );
      return {
        agentThreads: {
          ...s.agentThreads,
          [threadId]: {
            ...thread,
            messages: kept,
            checkpoints: [],
            selectedDiffTurn: null,
            total_additions: 0,
            total_deletions: 0,
          },
        },
        expandedArtifactId: null,
      };
    }),
});
