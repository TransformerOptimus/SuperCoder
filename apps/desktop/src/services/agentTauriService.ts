import { invoke } from '@tauri-apps/api/core';
import type {
  LlmConfig,
  AgentDisplayMessage,
  CheckpointSummary,
  ModelProfile,
  SessionRow,
} from '../types/agent';
import type { Attachment } from '../types/chat';
import type {
  SendMessageResponse,
  LlmConfigResponse,
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
  /** Create a session on a folder. Mode is switchable later, per message. */
  async createSession(folder: string, title?: string, mode?: string): Promise<SessionRow> {
    return invoke<SessionRow>('agent_create_session', {
      folder,
      title: title ?? null,
      mode: mode ?? null,
    });
  },

  async listSessions(): Promise<SessionRow[]> {
    return invoke<SessionRow[]>('agent_list_sessions');
  },

  async renameSession(sessionId: string, title: string): Promise<void> {
    return invoke<void>('agent_rename_session', { sessionId, title });
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
  ): Promise<{ total_tokens: number; context_limit: number; message_count: number } | null> {
    return invoke<{ total_tokens: number; context_limit: number; message_count: number } | null>(
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

  // ── LLM config / models ────────────────────────────────────────────────
  async fetchModels(): Promise<ModelProfile[]> {
    return invoke<ModelProfile[]>('agent_fetch_models');
  },

  async saveLlmConfig(config: LlmConfig): Promise<void> {
    return invoke<void>('agent_save_llm_config', {
      baseUrl: config.baseUrl,
      apiKey: config.apiKey,
      model: config.model,
    });
  },

  async getLlmConfig(): Promise<LlmConfig> {
    const res = await invoke<LlmConfigResponse>('agent_get_llm_config');
    return { baseUrl: res.base_url, apiKey: res.api_key, model: res.model };
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
