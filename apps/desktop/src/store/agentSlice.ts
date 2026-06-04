import type { StateCreator } from 'zustand';
import type { AgentToolCallState, ProviderConfig, ModelSelection, ModelCapability, ContextWatcherStatus, EngineStatus } from '../types/agent';
import type { PendingApproval, TodoItem } from '../types/agentContract';

// ── Types ──────────────────────────────────────────────────────────────────

export interface ActiveTool {
  toolCallId: string;
  toolName: string;
  argsSummary: string;
}

export interface AgentStreamingState {
  isStreaming: boolean;
  textBuffer: string;
  activeTool: ActiveTool | null;
  toolCalls: AgentToolCallState[];
  startedAt: number | null;
  error: string | null;
  totalTokens: number;
  contextLimit: number;
}

export interface AgentSlice {
  // ── Folder / branch selection for new sessions ───────────────────────
  agentFolderPath: string | null;
  agentBranch: string | null;

  /** Per-session streaming state. Key = session_id. */
  agentStreaming: Record<string, AgentStreamingState>;
  pendingApprovals: Record<string, PendingApproval>;
  /** Maps session_id → the running loop's session id (same value today). */
  activeSessionIds: Record<string, string>;
  /** Per-session pending question from the agent's ask_user tool. */
  pendingQuestions: Record<string, { sessionId: string; question: string; options?: string[] }>;
  /** Per-session agent todo list from the todo_write tool. */
  agentTodos: Record<string, TodoItem[]>;
  /** Per-session token usage (persists after streaming clears). */
  tokenUsage: Record<string, { totalTokens: number; contextLimit: number | null; cacheReadTokens?: number; cacheCreationTokens?: number }>;
  /** Live file-watcher status per repo path, fed by `context-watcher-status` events. */
  contextWatcherStatus: Record<string, ContextWatcherStatus>;
  /** App-managed engine lifecycle status, fed by `engine:status` events (null until known). */
  engineStatus: EngineStatus | null;
  /** Latest `docker compose` progress line, fed by `engine:progress` events. */
  engineProgress: string | null;

  // ── Plan-mode flow ──────────────────────────────────────────────────
  /** Per-project completed plans from plan-mode sessions. Key = projectPath. */
  completedPlans: Record<string, { text: string; projectPath: string; planPath: string }>;
  /** Which project's plan is expanded in the side panel (null = closed). */
  activePlanProjectPath: string | null;
  /** Transient: plan data waiting for coding session confirmation after Implement click. */
  pendingPlanForCoding: { text: string; projectPath: string; planPath: string } | null;

  // ── Provider state ──────────────────────────────────────────────────
  providers: ProviderConfig[];
  selection: ModelSelection;
  providersLoaded: boolean;
  /** Resolved capability (context limit + vision) for the active model. */
  activeCapability: ModelCapability | null;

  // ── Actions ─────────────────────────────────────────────────────────
  setAgentFolderPath: (path: string | null) => void;
  setAgentBranch: (branch: string | null) => void;

  setAgentStreaming: (sessionId: string, state: AgentStreamingState) => void;
  appendTextDelta: (sessionId: string, delta: string) => void;
  setActiveTool: (sessionId: string, tool: ActiveTool | null) => void;
  addToolCall: (sessionId: string, toolCall: AgentToolCallState) => void;
  updateToolCall: (sessionId: string, toolCallId: string, success: boolean, summary: string) => void;
  setStreamingError: (sessionId: string, error: string | null) => void;
  setTokenUsage: (sessionId: string, totalTokens: number, contextLimit: number | null, cacheReadTokens?: number, cacheCreationTokens?: number) => void;
  clearTokenUsage: (sessionId: string) => void;
  setContextWatcherStatus: (repoPath: string, status: ContextWatcherStatus) => void;
  setEngineStatus: (status: EngineStatus) => void;
  setEngineProgress: (line: string) => void;
  clearAgentStreaming: (sessionId: string) => void;
  softClearAgentStreaming: (sessionId: string) => void;

  setActiveSession: (sessionId: string, loopId: string) => void;
  clearActiveSession: (sessionId: string) => void;

  addPendingApproval: (approval: PendingApproval) => void;
  removePendingApproval: (toolCallId: string) => void;

  setPendingQuestion: (sessionId: string, data: { sessionId: string; question: string; options?: string[] }) => void;
  clearPendingQuestion: (sessionId: string) => void;

  setAgentTodos: (sessionId: string, todos: TodoItem[]) => void;

  setCompletedPlan: (projectPath: string, plan: { text: string; projectPath: string; planPath: string } | null) => void;
  clearCompletedPlan: (projectPath: string) => void;
  setActivePlanProjectPath: (path: string | null) => void;
  setPendingPlanForCoding: (plan: { text: string; projectPath: string; planPath: string } | null) => void;

  loadProviders: () => Promise<void>;
  setActiveModel: (providerId: string, model: string) => Promise<void>;
  /** Resolve and cache the active model's capability (context limit + vision). */
  refreshActiveCapability: () => Promise<void>;
}

// ── Helpers ────────────────────────────────────────────────────────────────

const emptyStreamingState: AgentStreamingState = {
  isStreaming: false,
  textBuffer: '',
  activeTool: null,
  toolCalls: [],
  startedAt: null,
  error: null,
  totalTokens: 0,
  contextLimit: 0,
};

/** Factory for a fresh streaming entry (isStreaming=true, startedAt=now). */
export function createInitialStreamingState(): AgentStreamingState {
  return {
    isStreaming: true,
    textBuffer: '',
    activeTool: null,
    toolCalls: [],
    startedAt: Date.now(),
    error: null,
    totalTokens: 0,
    contextLimit: 0,
  };
}

/** Patch a single field on agentStreaming[sessionId], creating the entry if missing. */
function patchStreaming(
  s: AgentSlice,
  sessionId: string,
  patch: Partial<AgentStreamingState>,
  fallback: AgentStreamingState = { ...emptyStreamingState, isStreaming: true },
): Pick<AgentSlice, 'agentStreaming'> {
  const current = s.agentStreaming[sessionId] ?? fallback;
  return {
    agentStreaming: { ...s.agentStreaming, [sessionId]: { ...current, ...patch } },
  };
}

// ── Slice Creator ──────────────────────────────────────────────────────────

export const createAgentSlice: StateCreator<AgentSlice, [], [], AgentSlice> = (set, get) => ({
  agentFolderPath: null,
  agentBranch: null,
  agentStreaming: {},
  pendingApprovals: {},
  activeSessionIds: {},
  pendingQuestions: {},
  agentTodos: {},
  tokenUsage: {},
  contextWatcherStatus: {},
  engineStatus: null,
  engineProgress: null,
  completedPlans: {},
  activePlanProjectPath: null,
  pendingPlanForCoding: null,
  providers: [],
  selection: { active: null, compaction: null, title: null },
  providersLoaded: false,
  activeCapability: null,

  setAgentFolderPath: (path) => set({ agentFolderPath: path }),
  setAgentBranch: (branch) => set({ agentBranch: branch }),

  setAgentStreaming: (sessionId, state) =>
    set((s) => ({ agentStreaming: { ...s.agentStreaming, [sessionId]: state } })),

  appendTextDelta: (sessionId, delta) =>
    set((s) => {
      const current = s.agentStreaming[sessionId] ?? { ...emptyStreamingState, isStreaming: true };
      return patchStreaming(s, sessionId, { textBuffer: current.textBuffer + delta });
    }),

  setActiveTool: (sessionId, tool) =>
    set((s) => patchStreaming(s, sessionId, { activeTool: tool })),

  addToolCall: (sessionId, toolCall) =>
    set((s) => {
      const current = s.agentStreaming[sessionId] ?? { ...emptyStreamingState, isStreaming: true };
      return patchStreaming(s, sessionId, { toolCalls: [...current.toolCalls, toolCall] });
    }),

  updateToolCall: (sessionId, toolCallId, success, summary) =>
    set((s) => {
      const current = s.agentStreaming[sessionId];
      if (!current) return s;
      return {
        agentStreaming: {
          ...s.agentStreaming,
          [sessionId]: {
            ...current,
            toolCalls: current.toolCalls.map((tc) =>
              tc.toolCallId === toolCallId
                ? { ...tc, status: success ? ('success' as const) : ('error' as const), summary }
                : tc,
            ),
          },
        },
      };
    }),

  setStreamingError: (sessionId, error) =>
    set((s) => patchStreaming(s, sessionId, { isStreaming: false, error }, { ...emptyStreamingState })),

  setTokenUsage: (sessionId, totalTokens, contextLimit, cacheReadTokens, cacheCreationTokens) =>
    set((s) => ({
      tokenUsage: { ...s.tokenUsage, [sessionId]: { totalTokens, contextLimit, cacheReadTokens, cacheCreationTokens } },
    })),

  clearTokenUsage: (sessionId) =>
    set((s) => {
      const { [sessionId]: _, ...rest } = s.tokenUsage;
      return { tokenUsage: rest };
    }),

  setContextWatcherStatus: (repoPath, status) =>
    set((s) => ({
      contextWatcherStatus: { ...s.contextWatcherStatus, [repoPath]: status },
    })),

  setEngineStatus: (status) => set({ engineStatus: status }),
  setEngineProgress: (line) => set({ engineProgress: line }),

  clearAgentStreaming: (sessionId) =>
    set((s) => {
      const { [sessionId]: _, ...rest } = s.agentStreaming;
      return { agentStreaming: rest };
    }),

  softClearAgentStreaming: (sessionId) =>
    set((s) => {
      const current = s.agentStreaming[sessionId];
      if (!current) return s;
      return {
        agentStreaming: {
          ...s.agentStreaming,
          [sessionId]: { ...current, isStreaming: false, activeTool: null },
        },
      };
    }),

  setActiveSession: (sessionId, loopId) =>
    set((s) => ({ activeSessionIds: { ...s.activeSessionIds, [sessionId]: loopId } })),

  clearActiveSession: (sessionId) =>
    set((s) => {
      const { [sessionId]: _, ...rest } = s.activeSessionIds;
      return { activeSessionIds: rest };
    }),

  addPendingApproval: (approval) =>
    set((s) => ({ pendingApprovals: { ...s.pendingApprovals, [approval.toolCallId]: approval } })),

  removePendingApproval: (toolCallId) =>
    set((s) => {
      const { [toolCallId]: _, ...rest } = s.pendingApprovals;
      return { pendingApprovals: rest };
    }),

  setPendingQuestion: (sessionId, data) =>
    set((s) => ({ pendingQuestions: { ...s.pendingQuestions, [sessionId]: data } })),

  clearPendingQuestion: (sessionId) =>
    set((s) => {
      const { [sessionId]: _, ...rest } = s.pendingQuestions;
      return { pendingQuestions: rest };
    }),

  setAgentTodos: (sessionId, todos) =>
    set((s) => ({ agentTodos: { ...s.agentTodos, [sessionId]: todos } })),

  setCompletedPlan: (projectPath, plan) =>
    set((s) => {
      if (plan) {
        return { completedPlans: { ...s.completedPlans, [projectPath]: plan } };
      }
      const { [projectPath]: _, ...rest } = s.completedPlans;
      return { completedPlans: rest };
    }),
  clearCompletedPlan: (projectPath) =>
    set((s) => {
      const { [projectPath]: _, ...rest } = s.completedPlans;
      return { completedPlans: rest };
    }),
  setActivePlanProjectPath: (path) => set({ activePlanProjectPath: path }),
  setPendingPlanForCoding: (plan) => set({ pendingPlanForCoding: plan }),

  loadProviders: async () => {
    try {
      const { agentTauriService } = await import('../services/agentTauriService');
      const { providers, selection } = await agentTauriService.listProviders();
      set({ providers, selection, providersLoaded: true });
      void get().refreshActiveCapability();
    } catch (e) {
      console.error('[agentSlice] Failed to load providers:', e);
    }
  },

  setActiveModel: async (providerId, model) => {
    set((s) => ({ selection: { ...s.selection, active: { providerId, model } } }));
    try {
      const { agentTauriService } = await import('../services/agentTauriService');
      await agentTauriService.setModelSelection('active', providerId, model);
      void get().refreshActiveCapability();
    } catch (e) {
      console.error('[agentSlice] Failed to set active model:', e);
    }
  },

  refreshActiveCapability: async () => {
    const active = get().selection.active;
    if (!active) {
      set({ activeCapability: null });
      return;
    }
    try {
      const { agentTauriService } = await import('../services/agentTauriService');
      const cap = await agentTauriService.resolveModelCapability(active.providerId, active.model);
      set({ activeCapability: cap });
    } catch (e) {
      console.error('[agentSlice] Failed to resolve model capability:', e);
      set({ activeCapability: null });
    }
  },
});
