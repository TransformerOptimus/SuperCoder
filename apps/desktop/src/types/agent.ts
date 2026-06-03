// ============================================================
// Agent TypeScript interfaces
// ============================================================

// --- Agent (same shape as WorkspaceUser from Rails get_users?filter=agents) ---

export interface Agent {
  id: number;
  agent_id?: string;
  name: string;
  email: string;
  first_name?: string;
  last_name?: string;
  avatar_url?: string;
  description?: string;
  agent_type?: string;
}

// ============================================================
// Agent Chat — Diffs, Artifacts, Messages, Threads
// ============================================================

// --- Diff primitives ---

export type FileDiffStatus = 'added' | 'modified' | 'deleted' | 'renamed';

export interface DiffLine {
  type: 'add' | 'delete' | 'context';
  content: string;
  old_line_number?: number;
  new_line_number?: number;
}

export interface DiffHunk {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  header: string;
  lines: DiffLine[];
}

export interface FileDiff {
  file_path: string;
  old_path?: string;
  status: FileDiffStatus;
  additions: number;
  deletions: number;
  hunks: DiffHunk[];
}

// --- Generic Artifact System ---

export type ArtifactType = 'code_changes' | 'terminal' | 'file' | 'text';

export interface BaseArtifact {
  id: string;
  type: ArtifactType;
  name: string;
  created_at: string;
}

export interface CodeChangesArtifact extends BaseArtifact {
  type: 'code_changes';
  files: FileDiff[];
  total_additions: number;
  total_deletions: number;
  files_changed?: number;
  /** Turn number this diff belongs to (undefined = cumulative/legacy). */
  turnCount?: number;
}

export interface TerminalArtifact extends BaseArtifact {
  type: 'terminal';
  command: string;
  output: string;
  exit_code: number;
}

export interface FileArtifact extends BaseArtifact {
  type: 'file';
  url: string;
  file_name: string;
  media_type: string;
  size?: number;
}

export interface TextArtifact extends BaseArtifact {
  type: 'text';
  content: string;
  format?: 'plain' | 'markdown' | 'html';
}

export type Artifact = CodeChangesArtifact | TerminalArtifact | FileArtifact | TextArtifact;

// --- Agent Messages ---

export type AgentMessageRole = 'user' | 'agent';

export interface AgentMessage {
  id: string;
  thread_id: string;
  agent_id: string;
  role: AgentMessageRole;
  text: string;
  /** Image data-URLs attached to this message (shown in the bubble). */
  images?: string[];
  artifacts: Artifact[];
  created_at: string;
  thinking?: {
    toolCalls: { toolCallId: string; toolName: string; argsSummary: string; status: string; summary?: string }[];
    durationSeconds: number;
  };
}

// --- Agent Thread ---

export interface AgentThread {
  id: string;
  agent_id: string;
  task_summary: string;
  folder_path: string;
  branch: string;
  worktree_path?: string;
  status: 'active' | 'completed' | 'error';
  is_coding_session: boolean;
  total_additions: number;
  total_deletions: number;
  /** Number of files changed in the working tree (from the git diff). */
  files_changed?: number;
  checkpoints: CheckpointSummary[];
  selectedDiffTurn: number | null;
  messages: AgentMessage[];
  /** Full plan text when this coding session was started from a plan. */
  sourcePlanText?: string;
  created_at: string;
  updated_at: string;
}

export interface CheckpointSummary {
  turn_count: number;
  checkpoint_ref: string;
  commit_sha: string;
  files: string[];
  additions: number;
  deletions: number;
  status: string;
  created_at: string;
}

// --- UI State ---

export type AgentViewMode = 'chat' | 'thread' | 'diff_review';

export interface ArtifactFileDecision {
  filePath: string;
  decision: 'pending' | 'accepted' | 'rejected';
}

// --- Agent Display Message (from SQLite via Tauri) ---

export interface AgentToolChip {
  name: string;
  summary: string;
}

export interface AgentDisplayMessage {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  created_at: string;
  session_id: string;
  /** Tool calls reconstructed from the SQLite table for the "Thought for…" chips. */
  tools: AgentToolChip[];
  /** Seconds spent on tool calls before this message (for "Thought for Ns"). */
  duration_seconds: number;
  /** Image data-URLs attached to this message (rebuilt from on-disk refs). */
  images: string[];
}

// --- Session (from the `sessions` table via Tauri) ---

export type SessionMode = 'ask' | 'plan' | 'coding';

export interface SessionRow {
  id: string;
  folder: string;
  mode: string;
  title: string | null;
  parent_session_id: string | null;
  created_at: string;
  updated_at: string;
  status: string;
  providerId?: string | null;
  model?: string | null;
}

// --- Thread Summary (from SQLite via Tauri) ---

export interface ThreadSummary {
  thread_id: string;
  is_coding_session: boolean;
  task_summary: string;
  project_path: string;
  branch: string;
  created_at: string;
  message_count: number;
}

export interface AgentToolCallState {
  toolCallId: string;
  toolName: string;
  argsSummary: string;
  status: 'running' | 'success' | 'error';
  summary?: string;
}

export type AgentSessionStatus = 'idle' | 'streaming' | 'tool_running' | 'done' | 'error';

export interface ModelProfile {
  id: string;
  display_name: string;
  provider: string;
  context_window: number;
}

/** UI provider kind → backend wire format. "openai_compatible" takes a custom base_url. */
export type ProviderKind = 'openai' | 'openai_compatible' | 'anthropic';

/** Per-model discovered/edited metadata. Mirrors Rust `ModelMeta`. */
export interface ModelMeta {
  /** Discovered context length; absent/null = unknown. */
  contextLength?: number | null;
  supportsImages?: boolean;
}

/** A saved LLM provider = an endpoint (no model bundled). Mirrors Rust `ProviderConfig`. */
export interface ProviderConfig {
  id: string;
  kind: ProviderKind;
  /** Display name — shown for openai_compatible providers; built-ins use their kind name. */
  label: string;
  baseUrl: string;
  apiKey: string;
  /** Available model ids (populated by "Fetch models" or typed). Feeds the pickers. */
  models: string[];
  /** Per-model metadata keyed by model id (discovered context length, vision). */
  modelMeta?: Record<string, ModelMeta>;
  /** Provider-level vision fallback for custom providers. */
  supportsImages?: boolean;
}

/** A model advertised by a provider's /models, with discovered context length. */
export interface FetchedModel {
  id: string;
  contextLength: number | null;
}

/** A built-in registry model for the Settings picker. Mirrors Rust `CuratedModel`. */
export interface CuratedModel {
  id: string;
  displayName: string;
  /** "openai" | "anthropic" — matches the built-in provider kind. */
  provider: string;
  contextWindow: number;
  supportsImages: boolean;
}

/** Resolved capability for the active (provider, model). Mirrors Rust `ModelCapability`. */
export interface ModelCapability {
  /** `null` = unknown → context bar shows raw count, auto-compaction off. */
  contextLimit: number | null;
  supportsImages: boolean;
  /** "known" | "discovered" | "unknown". */
  source: string;
}

/** A model on a specific provider. */
export interface ModelRef {
  providerId: string;
  model: string;
}

/** Global model selections — each picks a model from across configured providers. */
export interface ModelSelection {
  active: ModelRef | null;
  compaction: ModelRef | null;
  title: ModelRef | null;
}

export type SelectionRole = 'active' | 'compaction' | 'title';

export interface ProvidersResponse {
  providers: ProviderConfig[];
  selection: ModelSelection;
}

/** Opt-in context engine (semantic + graph code search) settings. */
export interface ContextEngineSettings {
  enabled: boolean;
  port: number;
}

// --- Agent List API ---

export interface ListAgentsParams {
  page?: number;
  size?: number;
  status?: string;
  query?: string;
}

export interface AgentListResponse {
  success: boolean;
  agents: Agent[];
  page: number;
  size: number;
  total: number;
  hasMore: boolean;
}

// --- Agent Chat API Payloads/Responses ---

export interface SendAgentMessagePayload {
  agent_id: string;
  text: string;
  folder_path: string;
  branch: string;
  thread_id?: string;
}

export interface SendAgentMessageResponse {
  success: boolean;
  thread: AgentThread;
}

export interface AgentThreadResponse {
  success: boolean;
  thread: AgentThread;
}

export interface AgentThreadListResponse {
  success: boolean;
  threads: AgentThread[];
  page: number;
  size: number;
  total: number;
  hasMore: boolean;
}
