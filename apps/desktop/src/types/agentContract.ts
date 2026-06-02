// ============================================================
// Agent Contract Types — TypeScript mirror of Rust/Tauri interface
// Source of truth: docs/frontend-agent-contract.md
// All fields use snake_case to match Rust serialization.
// ============================================================

// ── Command Parameter Types ─────────────────────────────────

export interface AgentSendMessageParams {
  message: string;
  working_dir?: string | null;
  branch?: string | null;
  session_id?: string | null;
  /** Agent mode: 'ask' (default) or 'plan' (read-only + ask_user + todo_write). */
  mode?: 'ask' | 'plan' | null;
}

export interface AgentSendThreadMessageParams {
  thread_id: string;
  text: string;
  project_path?: string | null;
}

export interface AgentResumeSessionParams {
  thread_id: string;
  message: string;
  project_path?: string | null;
}

export interface AgentCancelSessionParams {
  session_id: string;
}

export interface AgentApproveToolParams {
  session_id: string;
  tool_call_id: string;
  approved: boolean;
}

export interface AgentGetMessagesParams {
  thread_id?: string | null;
}

export interface AgentGetDiffParams {
  project_path: string;
  branch?: string | null;
}

export interface AgentGetWorkingDiffParams {
  project_path: string;
  files?: string[] | null;
}

export interface AgentSaveLlmConfigParams {
  base_url: string;
  api_key: string;
  model: string;
}

// ── Command Response Types ──────────────────────────────────

export interface SendMessageResponse {
  session_id: string;
  /** Raw checkpoint rows (Bug 16 hydration). Present only when starting/resuming
   * a coding thread; null/absent for ask-mode and plan-mode responses. */
  checkpoints?: Array<{
    id: number;
    thread_id: string;
    turn_count: number;
    checkpoint_ref: string;
    commit_sha: string;
    files_json: string;
    additions: number;
    deletions: number;
    status: string;
    created_at: string;
  }>;
  total_additions?: number;
  total_deletions?: number;
}

export interface AgentDiffResult {
  files_changed: number;
  insertions: number;
  deletions: number;
  stat: string;
  diff: string;
}

export interface LlmConfigResponse {
  base_url: string;
  api_key: string;
  model: string;
}

export interface SessionListItem {
  id: string;
  status: string;
  mode: string;
  project_path: string | null;
  branch: string | null;
}

// ── Event Payload Types ─────────────────────────────────────

export interface AgentConfigPayload {
  agent_user_id: number;
  dm_group_id: number;
}

export interface TextDeltaPayload {
  thread_id: string;
  delta: string;
}

export interface ToolStartPayload {
  thread_id: string;
  tool_call_id: string;
  tool_name: string;
  args_summary: string;
}

export interface ToolEndPayload {
  thread_id: string;
  tool_call_id: string;
  success: boolean;
  summary: string;
  modified_files?: string[];
}

export interface ApprovalNeededPayload {
  thread_id: string;
  tool_call_id: string;
  tool_name: string;
  description: string;
  args?: Record<string, unknown>;
}

export interface PendingApproval {
  threadId: string;
  toolCallId: string;
  toolName: string;
  description: string;
  /** Raw command/pattern for bash, grep, git tools */
  rawCommand?: string;
  /** Raw tool arguments for diff preview (edit/write tools) */
  args?: Record<string, unknown>;
}

export interface MessageCompletePayload {
  thread_id: string;
  message?: { id?: string; role?: string; content?: string; created_at?: string };
  /** Legacy format: raw content string. */
  content?: string;
}

export interface SessionStartedPayload {
  thread_id: string;
  project_path: string;
  branch: string | null;
  task_summary: string;
}

export interface DonePayload {
  thread_id: string;
  summary: string | null;
}

export interface ErrorPayload {
  thread_id: string;
  message: string;
  retrying: boolean;
}

export interface CompactionPayload {
  thread_id: string;
}

export interface TokenUsagePayload {
  thread_id: string;
  total_tokens: number;
  context_limit: number;
  cache_read_tokens?: number;
  cache_creation_tokens?: number;
}

export interface TurnDiffCompletedPayload {
  thread_id: string;
  turn_count: number;
  files: string[];
  additions: number;
  deletions: number;
  diff?: string;
  stat?: string;
  status: string;
}

export interface CheckpointRestoredPayload {
  thread_id: string;
  turn_count: number;
}

export interface SessionCompletePayload {
  session_id: string;
  project_path: string;
  mode: string;
  task_summary: string;
}

export interface QuestionAskedPayload {
  thread_id: string;
  session_id: string;
  question: string;
  options?: string[] | null;
}

export interface PlanReadyPayload {
  thread_id: string;
  session_id: string;
  plan: string;
  plan_path: string;
  project_path: string;
}

export interface TodoUpdatedPayload {
  thread_id: string;
  todos: TodoItem[];
}

export interface TodoItem {
  id: string;
  content: string;
  status: 'pending' | 'in_progress' | 'completed';
}

export interface SkillLoadedPayload {
  thread_id: string;
  skill_name: string;
}

export interface SkillListEntry {
  name: string;
  description: string;
  origin: 'default' | 'global' | 'project';
  enabled: boolean;
  path: string;
}

export interface SkillsPaths {
  global: string;
  project: string | null;
}

export interface SubagentListEntry {
  name: string;
  description: string;
  origin: 'default' | 'global' | 'project';
  enabled: boolean;
  // Note: Rust-side DialogEntry carries `#[serde(rename_all = "camelCase")]`,
  // so `allowed_tools` serializes as `allowedTools` over the wire. Keep the
  // camelCase name here even though most other contract types are snake_case.
  allowedTools: string[] | null;
  model: string | null;
  path: string;
}

export interface SubagentsPaths {
  global: string;
  project: string | null;
}

export interface SubagentStartPayload {
  thread_id: string;
  parent_tool_call_id: string;
  child_session_id: string;
  subagent_name: string;
  /** First ~120 chars of the prompt, whitespace-collapsed, truncated with … */
  prompt_preview: string;
}

export interface SubagentEndPayload {
  thread_id: string;
  parent_tool_call_id: string;
  child_session_id: string;
  success: boolean;
  summary: string;
}

// ── Display Types (from Rust serialization) ─────────────────

export interface DisplayMessage {
  id: string;
  workspace_id: number;
  recipient_type: number;
  recipient_id: number;
  thread_id: string;
  user_id: number;
  text: string;
  sent_at: string;
  sender: DisplaySender;
  attachments: never[];
  mentions: never[];
  mention_groups: never[];
  reactions: Record<string, never>;
  my_reactions: Record<string, never>;
  pinned: boolean;
  task_id: string;
  reply_count: number;
  broadcast_from_thread_id: string | null;
  also_sent_to_channel: boolean;
  updated_at: string;
  edited: boolean;
  edited_at: string | null;
}

export interface DisplaySender {
  id: number;
  first_name: string;
  last_name: string;
  email: string;
  displayName: string;
  initials: string;
  avatarColor: string;
}

// ── Permission Types ────────────────────────────────────────

export type PermissionLevel = 'AutoApproveAll' | 'ApproveDestructive' | 'ApproveEverything';

export interface ToolOverrides {
  auto_approve: string[];
  always_ask: string[];
}

export interface PermissionConfig {
  project_path: string | null;
  level: PermissionLevel;
  tool_overrides: ToolOverrides | null;
}
