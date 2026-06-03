use serde::Serialize;

/// Events emitted by the agent loop for UI consumption.
#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type")]
pub enum AgentEvent {
    TextDelta {
        session_id: String,
        delta: String,
    },
    ThinkingDelta {
        session_id: String,
        delta: String,
    },
    ToolStart {
        session_id: String,
        tool_call_id: String,
        tool_name: String,
        args_summary: String,
    },
    ToolStatus {
        session_id: String,
        tool_call_id: String,
        status: String,
    },
    ToolEnd {
        session_id: String,
        tool_call_id: String,
        success: bool,
        summary: String,
        /// Files modified by this tool (for write/edit tools).
        #[serde(skip_serializing_if = "Option::is_none")]
        modified_files: Option<Vec<String>>,
    },
    Error {
        session_id: String,
        message: String,
        retrying: bool,
    },
    Done {
        session_id: String,
        summary: Option<String>,
    },
    Compaction {
        session_id: String,
    },
    /// Token usage update after each LLM call.
    TokenUsage {
        session_id: String,
        total_tokens: u32,
        /// Known/discovered context window. `None` for models with an unknown
        /// limit — the UI then shows the raw token count with no max/percentage.
        context_limit: Option<u32>,
        /// Tokens served from prompt cache this call (read tier).
        /// None when caching isn't active or the provider didn't report it.
        cache_read_tokens: Option<u32>,
        /// Tokens written to prompt cache this call (write tier, Anthropic).
        cache_creation_tokens: Option<u32>,
    },
    /// Fired at the end of each agent loop iteration (after all tool calls complete).
    /// Carries the turn number and list of files modified during this turn.
    TurnCompleted {
        session_id: String,
        turn_count: u32,
        modified_files: Vec<String>,
    },
    /// The agent is asking the user a clarifying question (yield).
    UserQuestionAsked {
        session_id: String,
        question: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        options: Option<Vec<String>>,
    },
    /// The agent updated its todo list.
    TodoUpdated {
        session_id: String,
        todos: Vec<TodoItem>,
    },
    /// The plan-mode agent saved an implementation plan (yield).
    PlanReady {
        session_id: String,
        plan: String,
        plan_path: String,
        project_path: String,
    },
    /// The `skill` tool loaded a skill body into the conversation.
    SkillLoaded {
        session_id: String,
        skill_name: String,
    },
    /// A `spawn_subagent` tool call has started a child AgentLoop. Emitted on
    /// the PARENT's event_tx. Per DEC-1, the child's stream is NOT forwarded
    /// to the parent UI — this event (plus SubagentEnd) is the sole parent-
    /// visible representation. `prompt_preview` is the first ~120 chars of
    /// the user-facing prompt, truncated for chip display.
    SubagentStart {
        session_id: String,
        parent_tool_call_id: String,
        child_session_id: String,
        subagent_name: String,
        prompt_preview: String,
    },
    /// The child AgentLoop finished. `success` is false on error or cancel;
    /// `summary` is the child's final assistant text (or an error message).
    SubagentEnd {
        session_id: String,
        parent_tool_call_id: String,
        child_session_id: String,
        success: bool,
        summary: String,
    },
}

/// A single todo item tracked by the agent.
#[derive(Debug, Clone, Serialize, serde::Deserialize)]
pub struct TodoItem {
    pub id: String,
    pub content: String,
    pub status: String,
}

/// Result returned by the agent loop when it finishes.
#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type")]
pub enum AgentResult {
    /// Agent completed normally with a final summary.
    Done { summary: String },
    /// Agent is asking the user a clarifying question (yield from ask/plan mode).
    AskUser {
        question: String,
        options: Option<Vec<String>>,
    },
    /// Plan-mode agent saved a plan and yielded for user approval.
    PlanReady {
        plan: String,
        plan_path: String,
    },
}

impl AgentResult {
    /// Extract the summary from a Done result, panicking if it's not Done.
    /// Useful in tests.
    #[cfg(test)]
    pub fn unwrap_done(self) -> String {
        match self {
            AgentResult::Done { summary } => summary,
            other => panic!("Expected AgentResult::Done, got {:?}", other),
        }
    }
}
