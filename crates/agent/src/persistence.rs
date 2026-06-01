use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use std::sync::{Arc, Mutex};

/// Role of the message sender from the LLM perspective.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum MessageRole {
    User,
    Assistant,
    Tool,
    System,
}

/// Classification of message content.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum MessageType {
    Text,
    SessionInit,
    Compaction,
    ToolCall,
    ToolResult,
    CompletionSummary,
}

/// Who originated the message.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum Sender {
    HumanUser,
    Agent,
}

pub fn str_to_role(s: &str) -> MessageRole {
    match s {
        "user" => MessageRole::User,
        "assistant" => MessageRole::Assistant,
        "tool" => MessageRole::Tool,
        "system" => MessageRole::System,
        _ => {
            log::warn!("Unknown message role: '{s}', defaulting to User");
            MessageRole::User
        }
    }
}

pub fn str_to_type(s: &str) -> MessageType {
    match s {
        "text" => MessageType::Text,
        "session_init" => MessageType::SessionInit,
        "compaction" => MessageType::Compaction,
        "tool_call" => MessageType::ToolCall,
        "tool_result" => MessageType::ToolResult,
        "completion_summary" => MessageType::CompletionSummary,
        _ => {
            log::warn!("Unknown message type: '{s}', defaulting to Text");
            MessageType::Text
        }
    }
}

/// Persistence envelope — separate from ChatMessage (LLM wire format).
/// Wraps both human-readable content and the full LLM wire-format message.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentMessage {
    /// Human-readable content for display in chat UI.
    pub content: String,
    /// Full OpenAI wire-format message (ChatMessage serialized).
    pub llm_message: serde_json::Value,
    /// Operational metadata JSON.
    pub metadata: serde_json::Value,
    /// Role from the LLM perspective.
    pub role: MessageRole,
    /// Classification of this message.
    pub message_type: MessageType,
    /// Who originated this message.
    pub sender: Sender,
    /// Whether to also post this to the main DM channel.
    pub also_send_to_channel: bool,
    /// Which agent loop iteration (turn) produced this message.
    /// Set by the agent loop from its iteration counter.
    pub turn_count: Option<u32>,
}

/// Result of a successful persist operation.
#[derive(Debug, Clone)]
pub struct PersistResult {
    pub id: String,
}

/// Errors from persistence operations.
#[derive(Debug, thiserror::Error)]
pub enum PersistError {
    #[error("Storage error: {0}")]
    Storage(String),
    #[error("Not found: {0}")]
    NotFound(String),
}

/// Trait for persisting agent messages to external storage.
#[async_trait]
pub trait MessagePersister: Send + Sync {
    /// Persist a single message, optionally associated with a thread.
    async fn persist_message(
        &self,
        message: &AgentMessage,
        thread_id: Option<&str>,
    ) -> Result<PersistResult, PersistError>;

    /// Load all messages for a coding session (thread).
    async fn load_session_context(
        &self,
        thread_id: &str,
    ) -> Result<Vec<AgentMessage>, PersistError>;

    /// Load ask-mode context (messages not associated with any thread).
    async fn load_ask_context(&self) -> Result<Vec<AgentMessage>, PersistError>;
}

/// Builds a child persister that stamps `parent_thread_id` on every insert.
/// Implemented by the Tauri layer (SQLite-backed) so `spawn_subagent` stays
/// Tauri-agnostic. A child AgentLoop receives the `Arc<dyn MessagePersister>`
/// returned from `for_subagent` and writes to it as normal.
pub trait PersisterFactory: Send + Sync {
    fn for_subagent(&self, parent_thread_id: &str) -> Arc<dyn MessagePersister>;
}

/// In-memory mock persister for testing.
pub struct MockPersister {
    messages: Arc<Mutex<Vec<(Option<String>, AgentMessage)>>>,
}

impl MockPersister {
    pub fn new() -> Self {
        Self {
            messages: Arc::new(Mutex::new(Vec::new())),
        }
    }

    /// Access all stored (thread_id, message) pairs for test assertions.
    pub fn messages(&self) -> Vec<(Option<String>, AgentMessage)> {
        self.messages.lock().unwrap().clone()
    }
}

impl Default for MockPersister {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl MessagePersister for MockPersister {
    async fn persist_message(
        &self,
        message: &AgentMessage,
        thread_id: Option<&str>,
    ) -> Result<PersistResult, PersistError> {
        let id = uuid::Uuid::new_v4().to_string();
        self.messages
            .lock()
            .unwrap()
            .push((thread_id.map(String::from), message.clone()));
        Ok(PersistResult { id })
    }

    async fn load_session_context(
        &self,
        thread_id: &str,
    ) -> Result<Vec<AgentMessage>, PersistError> {
        let msgs = self
            .messages
            .lock()
            .unwrap()
            .iter()
            .filter(|(tid, _)| tid.as_deref() == Some(thread_id))
            .map(|(_, msg)| msg.clone())
            .collect();
        Ok(msgs)
    }

    async fn load_ask_context(&self) -> Result<Vec<AgentMessage>, PersistError> {
        let msgs = self
            .messages
            .lock()
            .unwrap()
            .iter()
            .filter(|(tid, _)| tid.is_none())
            .map(|(_, msg)| msg.clone())
            .collect();
        Ok(msgs)
    }
}

/// Discards every write and returns empty contexts. Used as a SessionManager
/// default when production callers always pass a per-call `persister_override`
/// — guarantees the default is never silently exercised in production.
pub struct NoopPersister;

#[async_trait]
impl MessagePersister for NoopPersister {
    async fn persist_message(
        &self,
        _message: &AgentMessage,
        _thread_id: Option<&str>,
    ) -> Result<PersistResult, PersistError> {
        Ok(PersistResult {
            id: String::new(),
        })
    }

    async fn load_session_context(
        &self,
        _thread_id: &str,
    ) -> Result<Vec<AgentMessage>, PersistError> {
        Ok(Vec::new())
    }

    async fn load_ask_context(&self) -> Result<Vec<AgentMessage>, PersistError> {
        Ok(Vec::new())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn make_message(content: &str, role: MessageRole, msg_type: MessageType) -> AgentMessage {
        AgentMessage {
            content: content.to_string(),
            llm_message: json!({"role": "user", "content": content}),
            metadata: json!({}),
            role,
            message_type: msg_type,
            sender: Sender::HumanUser,
            also_send_to_channel: false,
            turn_count: None,
        }
    }

    #[tokio::test]
    async fn test_persist_and_load_round_trip() {
        let persister = MockPersister::new();

        let msg = make_message("hello", MessageRole::User, MessageType::Text);
        let result = persister.persist_message(&msg, None).await.unwrap();
        assert!(!result.id.is_empty());

        let ask_msgs = persister.load_ask_context().await.unwrap();
        assert_eq!(ask_msgs.len(), 1);
        assert_eq!(ask_msgs[0].content, "hello");
    }

    #[tokio::test]
    async fn test_multiple_threads_isolated() {
        let persister = MockPersister::new();

        let msg_a = make_message("thread-a msg", MessageRole::User, MessageType::Text);
        let msg_b = make_message("thread-b msg", MessageRole::User, MessageType::Text);

        persister
            .persist_message(&msg_a, Some("thread-a"))
            .await
            .unwrap();
        persister
            .persist_message(&msg_b, Some("thread-b"))
            .await
            .unwrap();

        let a_msgs = persister.load_session_context("thread-a").await.unwrap();
        assert_eq!(a_msgs.len(), 1);
        assert_eq!(a_msgs[0].content, "thread-a msg");

        let b_msgs = persister.load_session_context("thread-b").await.unwrap();
        assert_eq!(b_msgs.len(), 1);
        assert_eq!(b_msgs[0].content, "thread-b msg");
    }

    #[tokio::test]
    async fn test_ask_context_excludes_thread_messages() {
        let persister = MockPersister::new();

        let ask_msg = make_message("ask msg", MessageRole::User, MessageType::Text);
        let thread_msg = make_message("thread msg", MessageRole::User, MessageType::Text);

        persister.persist_message(&ask_msg, None).await.unwrap();
        persister
            .persist_message(&thread_msg, Some("thread-1"))
            .await
            .unwrap();

        let ask_msgs = persister.load_ask_context().await.unwrap();
        assert_eq!(ask_msgs.len(), 1);
        assert_eq!(ask_msgs[0].content, "ask msg");
    }

    #[tokio::test]
    async fn test_compaction_records_found_by_type() {
        let persister = MockPersister::new();

        let text_msg = make_message("regular", MessageRole::User, MessageType::Text);
        let compact_msg = make_message("summary", MessageRole::System, MessageType::Compaction);

        persister
            .persist_message(&text_msg, Some("thread-1"))
            .await
            .unwrap();
        persister
            .persist_message(&compact_msg, Some("thread-1"))
            .await
            .unwrap();

        let msgs = persister.load_session_context("thread-1").await.unwrap();
        assert_eq!(msgs.len(), 2);

        let compaction_msgs: Vec<_> = msgs
            .iter()
            .filter(|m| m.message_type == MessageType::Compaction)
            .collect();
        assert_eq!(compaction_msgs.len(), 1);
        assert_eq!(compaction_msgs[0].content, "summary");
    }
}
