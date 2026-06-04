import { invoke } from '@tauri-apps/api/core';
import type {
  AgentDisplayMessage,
  CheckpointSummary,
  ContextEngineSettings,
  ContextEngineStatus,
  ContextWatcherStatus,
  EngineMode,
  EngineStatus,
  IndexedRepo,
  CuratedModel,
  FetchedModel,
  ModelCapability,
  ProviderConfig,
  ProvidersResponse,
  SelectionRole,
  SessionRow,
} from '../types/agent';
import type { Attachment } from '../types/chat';
import type {
  SendMessageResponse,
  AgentDiffResult,
  PermissionConfig,
  SkillListEntry,
  SkillsPaths,
  SubagentListEntry,
  SubagentsPaths,
} from '../types/agentContract';

/** Raw checkpoint info from the snapshot dir (git_ops::checkpoint::list). */
interface CheckpointInfo {
  turn: number;
  fileCount: number;
  paths: string[];
}

export const agentTauriService = {
  // ── Sessions ───────────────────────────────────────────────────────────
  /** Create a session on a folder. Mode is switchable later, per message.
   * provider/model default to the active selection when omitted. */
  async createSession(
    folder: string,
    title?: string,
    mode?: string,
    providerId?: string,
    model?: string,
  ): Promise<SessionRow> {
    return invoke<SessionRow>('agent_create_session', {
      folder,
      title: title ?? null,
      mode: mode ?? null,
      providerId: providerId ?? null,
      model: model ?? null,
    });
  },

  async listSessions(): Promise<SessionRow[]> {
    return invoke<SessionRow[]>('agent_list_sessions');
  },

  async renameSession(sessionId: string, title: string): Promise<void> {
    return invoke<void>('agent_rename_session', { sessionId, title });
  },

  /** Soft-delete a session (hidden from the list; data preserved). */
  async deleteSession(sessionId: string): Promise<void> {
    return invoke<void>('agent_delete_session', { sessionId });
  },

  /** Send a message. `mode` ("ask"|"plan"|"coding") can change on any message. */
  async sendMessage(
    sessionId: string,
    message: string,
    mode?: string,
    attachments?: Attachment[],
  ): Promise<SendMessageResponse> {
    return invoke<SendMessageResponse>('agent_send_message', {
      sessionId,
      message,
      mode: mode ?? null,
      attachments: attachments && attachments.length > 0 ? attachments : null,
    });
  },

  async startCodingFromPlan(
    projectPath: string,
    planText: string,
    planPath?: string,
    title?: string,
  ): Promise<SendMessageResponse> {
    return invoke<SendMessageResponse>('agent_start_coding_from_plan', {
      projectPath,
      planText,
      planPath: planPath ?? null,
      title: title ?? null,
    });
  },

  async cancelSession(sessionId: string): Promise<void> {
    return invoke<void>('agent_cancel_session', { sessionId });
  },

  async approveToolCall(sessionId: string, toolCallId: string, approved: boolean): Promise<void> {
    return invoke<void>('agent_approve_tool', { sessionId, toolCallId, approved });
  },

  async getMessages(sessionId: string): Promise<AgentDisplayMessage[]> {
    return invoke<AgentDisplayMessage[]>('agent_get_messages', { sessionId });
  },

  // ── Context usage / clear / compact ────────────────────────────────────
  async getContextUsage(
    sessionId: string,
  ): Promise<{ total_tokens: number; context_limit: number | null; message_count: number } | null> {
    return invoke<{ total_tokens: number; context_limit: number | null; message_count: number } | null>(
      'agent_get_context_usage',
      { sessionId },
    );
  },

  async clearContext(sessionId: string): Promise<void> {
    return invoke<void>('agent_clear_context', { sessionId });
  },

  async compactContext(sessionId: string): Promise<string> {
    return invoke<string>('agent_compact_context', { sessionId });
  },

  // ── Working-tree diffs ─────────────────────────────────────────────────
  async getDiff(projectPath: string): Promise<AgentDiffResult> {
    return invoke<AgentDiffResult>('agent_get_diff', { projectPath });
  },

  async getWorkingDiff(projectPath: string, files?: string[]): Promise<AgentDiffResult> {
    return invoke<AgentDiffResult>('agent_get_working_diff', {
      projectPath,
      files: files ?? null,
    });
  },

  // ── Permissions ────────────────────────────────────────────────────────
  async getPermissions(projectPath?: string | null): Promise<PermissionConfig> {
    return invoke<PermissionConfig>('agent_get_permissions', { projectPath: projectPath ?? null });
  },

  async setPermission(config: PermissionConfig): Promise<void> {
    return invoke<void>('agent_set_permission', { config });
  },

  // ── Skills / subagents ─────────────────────────────────────────────────
  async listSkills(workingDir?: string | null): Promise<SkillListEntry[]> {
    return invoke<SkillListEntry[]>('agent_list_skills', { workingDir: workingDir ?? null });
  },

  async setSkillEnabled(name: string, enabled: boolean): Promise<void> {
    return invoke<void>('agent_set_skill_enabled', { name, enabled });
  },

  async getSkillsPaths(workingDir?: string | null): Promise<SkillsPaths> {
    return invoke<SkillsPaths>('agent_get_skills_paths', { workingDir: workingDir ?? null });
  },

  async listSubagents(workingDir?: string | null): Promise<SubagentListEntry[]> {
    return invoke<SubagentListEntry[]>('agent_list_subagents', { workingDir: workingDir ?? null });
  },

  async setSubagentEnabled(name: string, enabled: boolean): Promise<void> {
    return invoke<void>('agent_set_subagent_enabled', { name, enabled });
  },

  async getSubagentsPaths(workingDir?: string | null): Promise<SubagentsPaths> {
    return invoke<SubagentsPaths>('agent_get_subagents_paths', { workingDir: workingDir ?? null });
  },

  // ── LLM providers (multi-provider config) ──────────────────────────────
  async listProviders(): Promise<ProvidersResponse> {
    return invoke<ProvidersResponse>('agent_list_providers');
  },

  async addProvider(provider: ProviderConfig): Promise<ProviderConfig> {
    return invoke<ProviderConfig>('agent_add_provider', { provider });
  },

  async updateProvider(provider: ProviderConfig): Promise<void> {
    return invoke<void>('agent_update_provider', { provider });
  },

  async deleteProvider(id: string): Promise<void> {
    return invoke<void>('agent_delete_provider', { id });
  },

  /** Set a global model selection: role ∈ "active" | "compaction" | "title". */
  async setModelSelection(role: SelectionRole, providerId: string, model: string): Promise<void> {
    return invoke<void>('agent_set_model_selection', { role, providerId, model });
  },

  /** Re-pin an open session's provider + model (picker switch while a session is open). */
  async setSessionModel(sessionId: string, providerId: string, model: string): Promise<void> {
    return invoke<void>('agent_set_session_model', { sessionId, providerId, model });
  },

  /** Query the provider's models endpoint (uses the draft's base_url/api_key/kind). */
  async fetchProviderModels(provider: ProviderConfig): Promise<FetchedModel[]> {
    return invoke<FetchedModel[]>('agent_fetch_provider_models', { provider });
  },

  /** Built-in model registry (context window + vision) — drives the Settings picker. */
  async listModels(): Promise<CuratedModel[]> {
    return invoke<CuratedModel[]>('agent_list_models');
  },

  /** Verify the provider's API key. Rejects (throws) only on a clear 401/403. */
  async verifyProvider(provider: ProviderConfig): Promise<void> {
    return invoke<void>('agent_verify_provider', { provider });
  },

  /** Resolve context limit + vision support for a (provider, model) pair. */
  async resolveModelCapability(providerId: string, model: string): Promise<ModelCapability> {
    return invoke<ModelCapability>('agent_resolve_model_capability', { providerId, model });
  },

  // ── Context engine (opt-in semantic/graph search) ──────────────────────
  async getContextEngine(): Promise<ContextEngineSettings> {
    return invoke<ContextEngineSettings>('agent_get_context_engine');
  },

  async setContextEngine(settings: ContextEngineSettings): Promise<void> {
    return invoke<void>('agent_set_context_engine', { settings });
  },

  /** Probe a backend URL (hits /api/health). Does not change saved settings. */
  async contextEngineStatus(baseUrl: string): Promise<ContextEngineStatus> {
    return invoke<ContextEngineStatus>('agent_context_engine_status', { baseUrl });
  },

  /** List known repos (session folders) + their index state on the backend. */
  async contextEngineRepos(): Promise<IndexedRepo[]> {
    return invoke<IndexedRepo[]>('agent_context_engine_repos');
  },

  /** Delete a repo's index (vectors + graph + merkle) on the backend. */
  async deleteContextEngineRepo(path: string): Promise<void> {
    return invoke<void>('agent_context_engine_delete_repo', { path });
  },

  /** Start the live file-watcher for a repo (idempotent: full sync + incremental). */
  async contextWatcherStart(repoPath: string): Promise<void> {
    return invoke<void>('context_watcher_start', { repoPath });
  },

  /** Stop the live file-watcher for a repo. */
  async contextWatcherStop(repoPath: string): Promise<void> {
    return invoke<void>('context_watcher_stop', { repoPath });
  },

  /** Query the current watcher status for a repo (null if not watched). */
  async contextWatcherStatus(repoPath: string): Promise<ContextWatcherStatus | null> {
    return invoke<ContextWatcherStatus | null>('context_watcher_status', { repoPath });
  },

  // ── App-managed engine lifecycle (app mode only) ───────────────────────
  /** Lifecycle mode: "user" (connect to a URL) or "app" (app runs the stack). */
  async engineMode(): Promise<EngineMode> {
    return invoke<EngineMode>('agent_engine_mode');
  },

  /** Current app-managed stack status (snapshot; live updates via engine:status). */
  async engineStatus(): Promise<EngineStatus> {
    return invoke<EngineStatus>('agent_engine_status');
  },

  /** Check Docker CLI + daemon + compose v2. Rejects with a message if unmet. */
  async enginePreflight(): Promise<void> {
    return invoke<void>('agent_engine_preflight');
  },

  /** Bring the app-managed stack up (pull + compose up + wait healthy). */
  async engineStart(): Promise<void> {
    return invoke<void>('agent_engine_start');
  },

  /** Stop the app-managed stack (keeps volumes). */
  async engineStop(): Promise<void> {
    return invoke<void>('agent_engine_stop');
  },

  /** Tear the stack down. `removeData` also drops the indexed-data volumes. */
  async engineRemove(removeData: boolean): Promise<void> {
    return invoke<void>('agent_engine_remove', { removeData });
  },

  /** Whether an embedding API key is stored for the app-managed stack. */
  async engineHasKey(): Promise<boolean> {
    return invoke<boolean>('agent_engine_has_key');
  },

  /** Persist the embedding API key (injected into the stack's process env). */
  async engineSetKey(key: string): Promise<void> {
    return invoke<void>('agent_engine_set_key', { key });
  },

  // ── Checkpoints (snapshot-backed) ──────────────────────────────────────
  async listCheckpoints(sessionId: string): Promise<CheckpointSummary[]> {
    const rows = await invoke<CheckpointInfo[]>('agent_list_checkpoints', { sessionId });
    return rows.map((row) => ({
      turn_count: row.turn,
      checkpoint_ref: `snapshot/${sessionId}/turn-${row.turn}`,
      commit_sha: '',
      files: row.paths,
      additions: 0,
      deletions: 0,
      status: 'ready',
      created_at: new Date().toISOString(),
    }));
  },

  async getTurnDiff(sessionId: string, turn: number): Promise<AgentDiffResult> {
    return invoke<AgentDiffResult>('agent_get_turn_diff', { sessionId, turn });
  },

  async getFullDiff(sessionId: string): Promise<AgentDiffResult> {
    return invoke<AgentDiffResult>('agent_get_full_diff', { sessionId });
  },

  async restoreCheckpoint(sessionId: string, turn: number): Promise<void> {
    return invoke<void>('agent_restore_checkpoint', { sessionId, turn });
  },

  async rewindToMessage(
    sessionId: string,
    messageSqliteId: number,
    restoreCode: boolean,
    newText: string,
    attachments?: Attachment[],
  ): Promise<{ session_id: string }> {
    return invoke<{ session_id: string }>('agent_rewind_to_message', {
      sessionId,
      messageSqliteId,
      restoreCode,
      newText,
      attachments: attachments && attachments.length > 0 ? attachments : null,
    });
  },

  async readFileText(path: string): Promise<string | null> {
    return invoke<string | null>('read_file_text', { path });
  },
};
