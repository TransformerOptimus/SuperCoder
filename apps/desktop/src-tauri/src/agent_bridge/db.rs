use std::path::Path;
use std::sync::Arc;

use async_trait::async_trait;
use parking_lot::Mutex;
use rusqlite::Connection;

use agent::persistence::{
    self, AgentMessage, MessagePersister, MessageRole, MessageType, PersistError, PersistResult,
    Sender,
};

// ── AgentDb ────────────────────────────────────────────────────────────────
//
// Greenfield single-user local store. Schema v1 — no migration ladder carried
// over from the chat product. Sessions are the unit of work (folder + mode);
// messages are keyed purely by `session_id`. Checkpoints live outside SQLite,
// in the file-snapshot dir managed by `git-ops`.

/// Local SQLite database for agent sessions + LLM message history.
pub struct AgentDb {
    conn: Mutex<Connection>,
}

/// A row from the `sessions` table — the unit shown in the session-list sidebar.
#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionRow {
    pub id: String,
    pub folder: String,
    /// "ask" | "plan" | "coding"
    pub mode: String,
    pub title: Option<String>,
    pub parent_session_id: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    /// "active" | "idle" | "error"
    pub status: String,
}

/// Raw row from the `agent_messages` table.
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct StoredMessage {
    pub id: i64,
    pub session_id: String,
    pub project_path: String,
    pub role: String,
    pub type_: String,
    pub llm_message: String,
    pub metadata: String,
    pub created_at: String,
    pub rewound_at: Option<String>,
    pub turn_count: Option<u32>,
}

/// Error type for AgentDb operations.
#[derive(Debug, thiserror::Error)]
pub enum AgentDbError {
    #[error("SQLite error: {0}")]
    Sqlite(#[from] rusqlite::Error),
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),
}

impl AgentDb {
    /// Open or create `agent_data.db` in the given directory with the v1 schema.
    pub fn new(data_dir: &Path) -> Result<Self, AgentDbError> {
        std::fs::create_dir_all(data_dir)?;
        let db_path = data_dir.join("agent_data.db");
        let conn = Connection::open(&db_path)?;

        conn.execute_batch(
            "PRAGMA journal_mode = WAL;
             PRAGMA busy_timeout = 5000;
             PRAGMA synchronous = NORMAL;
             PRAGMA wal_autocheckpoint = 400;
             PRAGMA cache_size = -32768;",
        )?;

        // Fresh-DB tuning: only set page_size before the first table is written.
        let initialized: bool = conn
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sessions'",
                [],
                |row| row.get::<_, i64>(0),
            )
            .map(|c| c > 0)?;
        if !initialized {
            conn.execute_batch("PRAGMA page_size = 8192;")?;
        }

        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS sessions (
                 id                TEXT PRIMARY KEY,
                 folder            TEXT NOT NULL,
                 mode              TEXT NOT NULL,
                 title             TEXT,
                 parent_session_id TEXT,
                 created_at        TEXT NOT NULL,
                 updated_at        TEXT NOT NULL,
                 status            TEXT NOT NULL DEFAULT 'active'
             );
             CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);
             CREATE INDEX IF NOT EXISTS idx_sessions_folder  ON sessions(folder);

             CREATE TABLE IF NOT EXISTS agent_messages (
                 id            INTEGER PRIMARY KEY AUTOINCREMENT,
                 session_id    TEXT NOT NULL DEFAULT '',
                 project_path  TEXT NOT NULL DEFAULT '',
                 role          TEXT NOT NULL,
                 type          TEXT NOT NULL,
                 llm_message   TEXT NOT NULL,
                 metadata      TEXT NOT NULL DEFAULT '{}',
                 created_at    TEXT NOT NULL,
                 rewound_at    TEXT DEFAULT NULL,
                 turn_count    INTEGER DEFAULT NULL
             );
             CREATE INDEX IF NOT EXISTS idx_msgs_session
                 ON agent_messages(session_id, created_at);
             CREATE INDEX IF NOT EXISTS idx_msgs_session_active
                 ON agent_messages(session_id, rewound_at);
             CREATE INDEX IF NOT EXISTS idx_msgs_type
                 ON agent_messages(session_id, type);

             CREATE TABLE IF NOT EXISTS skill_prefs (
                 skill_name TEXT PRIMARY KEY,
                 enabled    INTEGER NOT NULL DEFAULT 1
             );

             CREATE TABLE IF NOT EXISTS subagent_prefs (
                 subagent_name TEXT PRIMARY KEY,
                 enabled       INTEGER NOT NULL DEFAULT 1
             );

             PRAGMA user_version = 1;",
        )?;

        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    #[allow(dead_code)]
    pub fn schema_version(&self) -> Result<i32, AgentDbError> {
        let conn = self.conn.lock();
        Ok(conn.query_row("PRAGMA user_version", [], |row| row.get(0))?)
    }

    // ── Sessions ─────────────────────────────────────────────────────────

    /// Insert a new session row. `created_at`/`updated_at` are set to now.
    pub fn create_session(
        &self,
        id: &str,
        folder: &str,
        mode: &str,
        title: Option<&str>,
        parent_session_id: Option<&str>,
    ) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        let now = now_iso();
        conn.execute(
            "INSERT INTO sessions (id, folder, mode, title, parent_session_id, created_at, updated_at, status)
             VALUES (?, ?, ?, ?, ?, ?, ?, 'active')",
            rusqlite::params![id, folder, mode, title, parent_session_id, now, now],
        )?;
        Ok(())
    }

    /// Get a single session by id.
    pub fn get_session(&self, id: &str) -> Result<Option<SessionRow>, AgentDbError> {
        let conn = self.conn.lock();
        let result = conn.query_row(
            "SELECT id, folder, mode, title, parent_session_id, created_at, updated_at, status
             FROM sessions WHERE id = ?",
            [id],
            map_session_row,
        );
        match result {
            Ok(row) => Ok(Some(row)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    /// List all sessions, most-recently-updated first.
    pub fn list_sessions(&self) -> Result<Vec<SessionRow>, AgentDbError> {
        let conn = self.conn.lock();
        let mut stmt = conn.prepare(
            "SELECT id, folder, mode, title, parent_session_id, created_at, updated_at, status
             FROM sessions ORDER BY updated_at DESC",
        )?;
        let rows = stmt.query_map([], map_session_row)?;
        let mut out = Vec::new();
        for r in rows {
            out.push(r?);
        }
        Ok(out)
    }

    /// Set a session's status and bump `updated_at`.
    pub fn set_session_status(&self, id: &str, status: &str) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?",
            rusqlite::params![status, now_iso(), id],
        )?;
        Ok(())
    }

    /// Set a session's title (first message preview, etc.) and bump `updated_at`.
    pub fn set_session_title(&self, id: &str, title: &str) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?",
            rusqlite::params![title, now_iso(), id],
        )?;
        Ok(())
    }

    /// Bump a session's `updated_at` to now (recency for the sidebar).
    pub fn touch_session(&self, id: &str) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "UPDATE sessions SET updated_at = ? WHERE id = ?",
            rusqlite::params![now_iso(), id],
        )?;
        Ok(())
    }

    /// Return the id of an active session for the given folder, if any.
    /// Used to enforce one active session per folder.
    pub fn active_session_for_folder(&self, folder: &str) -> Result<Option<String>, AgentDbError> {
        let conn = self.conn.lock();
        let result = conn.query_row(
            "SELECT id FROM sessions WHERE folder = ? AND status = 'active' ORDER BY updated_at DESC LIMIT 1",
            [folder],
            |row| row.get::<_, String>(0),
        );
        match result {
            Ok(id) => Ok(Some(id)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    // ── Skill / subagent prefs ───────────────────────────────────────────

    pub fn load_disabled_skills(&self) -> Result<std::collections::HashSet<String>, AgentDbError> {
        let conn = self.conn.lock();
        let mut stmt = conn.prepare("SELECT skill_name FROM skill_prefs WHERE enabled = 0")?;
        let rows = stmt.query_map([], |r| r.get::<_, String>(0))?;
        Ok(rows.filter_map(Result::ok).collect())
    }

    pub fn set_skill_enabled(&self, name: &str, enabled: bool) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "INSERT INTO skill_prefs (skill_name, enabled) VALUES (?1, ?2)
             ON CONFLICT(skill_name) DO UPDATE SET enabled = excluded.enabled",
            rusqlite::params![name, enabled as i32],
        )?;
        Ok(())
    }

    pub fn load_disabled_subagents(
        &self,
    ) -> Result<std::collections::HashSet<String>, AgentDbError> {
        let conn = self.conn.lock();
        let mut stmt = conn.prepare("SELECT subagent_name FROM subagent_prefs WHERE enabled = 0")?;
        let rows = stmt.query_map([], |r| r.get::<_, String>(0))?;
        Ok(rows.filter_map(Result::ok).collect())
    }

    pub fn set_subagent_enabled(&self, name: &str, enabled: bool) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "INSERT INTO subagent_prefs (subagent_name, enabled) VALUES (?1, ?2)
             ON CONFLICT(subagent_name) DO UPDATE SET enabled = excluded.enabled",
            rusqlite::params![name, enabled as i32],
        )?;
        Ok(())
    }

    // ── Messages ─────────────────────────────────────────────────────────

    /// Insert a message for a session. Returns the row id as a string.
    #[allow(clippy::too_many_arguments)]
    pub fn insert_message(
        &self,
        session_id: &str,
        project_path: &str,
        role: &str,
        type_: &str,
        llm_message: &str,
        metadata: &str,
        turn_count: Option<u32>,
    ) -> Result<String, AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "INSERT INTO agent_messages (session_id, project_path, role, type, llm_message, metadata, created_at, turn_count)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            rusqlite::params![session_id, project_path, role, type_, llm_message, metadata, now_iso(), turn_count],
        )?;
        Ok(conn.last_insert_rowid().to_string())
    }

    /// Load all active (non-rewound) rows for a session, ordered by id ASC.
    pub fn load_session_messages(
        &self,
        session_id: &str,
    ) -> Result<Vec<StoredMessage>, AgentDbError> {
        let conn = self.conn.lock();
        let mut stmt = conn.prepare(
            "SELECT id, session_id, project_path, role, type, llm_message, metadata, created_at, rewound_at, turn_count
             FROM agent_messages
             WHERE session_id = ? AND rewound_at IS NULL
             ORDER BY id ASC",
        )?;
        let rows = stmt.query_map([session_id], map_stored_message)?;
        let mut out = Vec::new();
        for r in rows {
            out.push(r?);
        }
        Ok(out)
    }

    /// Load a session's messages optimized for LLM context (compaction-aware).
    /// Mirrors the historical behaviour: load from the last compaction marker
    /// (honoring `kept_before_count`), else fall back to the most recent
    /// `fallback_limit` rows; then trim leading orphaned tool rows.
    pub fn load_session_for_context(
        &self,
        session_id: &str,
        fallback_limit: u32,
    ) -> Result<Vec<StoredMessage>, AgentDbError> {
        let conn = self.conn.lock();

        let reset_floor: i64 = conn
            .query_row(
                "SELECT COALESCE(MAX(id), 0) FROM agent_messages
                 WHERE session_id = ? AND type = 'context_reset' AND rewound_at IS NULL",
                [session_id],
                |row| row.get(0),
            )
            .unwrap_or(0);

        let last_compaction_id: Option<i64> = conn
            .query_row(
                "SELECT MAX(id) FROM agent_messages
                 WHERE session_id = ? AND type = 'compaction' AND rewound_at IS NULL AND id > ?",
                rusqlite::params![session_id, reset_floor],
                |row| row.get(0),
            )
            .unwrap_or(None);

        let mut messages = if let Some(compaction_id) = last_compaction_id {
            let kept_before: i64 = conn
                .query_row(
                    "SELECT COALESCE(json_extract(metadata, '$.kept_before_count'), 0) FROM agent_messages WHERE id = ?",
                    [compaction_id],
                    |row| row.get(0),
                )
                .unwrap_or(0);

            let start_id: i64 = if kept_before > 0 {
                conn.query_row(
                    "SELECT COALESCE(MIN(id), ?) FROM (
                        SELECT id FROM agent_messages
                        WHERE session_id = ? AND id < ? AND id > ?
                          AND type NOT IN ('compaction', 'context_usage', 'context_reset')
                          AND rewound_at IS NULL
                        ORDER BY id DESC LIMIT ?
                    )",
                    rusqlite::params![compaction_id, session_id, compaction_id, reset_floor, kept_before],
                    |row| row.get(0),
                )
                .unwrap_or(compaction_id)
            } else {
                compaction_id
            };

            let mut stmt = conn.prepare(
                "SELECT id, session_id, project_path, role, type, llm_message, metadata, created_at, rewound_at, turn_count
                 FROM agent_messages
                 WHERE session_id = ? AND id >= ?
                   AND type NOT IN ('context_usage', 'context_reset')
                   AND rewound_at IS NULL
                 ORDER BY id ASC",
            )?;
            let rows = stmt.query_map(rusqlite::params![session_id, start_id], map_stored_message)?;
            let mut msgs = Vec::new();
            for r in rows {
                msgs.push(r?);
            }
            msgs
        } else {
            let mut stmt = conn.prepare(
                "SELECT id, session_id, project_path, role, type, llm_message, metadata, created_at, rewound_at, turn_count
                 FROM agent_messages
                 WHERE session_id = ? AND id > ?
                   AND type NOT IN ('context_usage', 'context_reset')
                   AND rewound_at IS NULL
                 ORDER BY id DESC LIMIT ?",
            )?;
            let rows = stmt.query_map(
                rusqlite::params![session_id, reset_floor, fallback_limit],
                map_stored_message,
            )?;
            let mut msgs = Vec::new();
            for r in rows {
                msgs.push(r?);
            }
            msgs.reverse();
            msgs
        };

        // Trim leading orphaned tool rows so the context starts clean.
        while let Some(first) = messages.first() {
            if first.role == "tool" || (first.role == "assistant" && first.type_ == "tool_call") {
                messages.remove(0);
            } else {
                break;
            }
        }

        Ok(messages)
    }

    /// Get the latest `session_init` record for a session (if any).
    pub fn get_session_init(
        &self,
        session_id: &str,
    ) -> Result<Option<StoredMessage>, AgentDbError> {
        let conn = self.conn.lock();
        let result = conn.query_row(
            "SELECT id, session_id, project_path, role, type, llm_message, metadata, created_at, rewound_at, turn_count
             FROM agent_messages
             WHERE session_id = ? AND type = 'session_init'
             ORDER BY id DESC LIMIT 1",
            [session_id],
            map_stored_message,
        );
        match result {
            Ok(msg) => Ok(Some(msg)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    /// Get a single message by SQLite row id.
    pub fn get_message_by_id(&self, id: i64) -> Result<Option<StoredMessage>, AgentDbError> {
        let conn = self.conn.lock();
        let result = conn.query_row(
            "SELECT id, session_id, project_path, role, type, llm_message, metadata, created_at, rewound_at, turn_count
             FROM agent_messages WHERE id = ?",
            [id],
            map_stored_message,
        );
        match result {
            Ok(msg) => Ok(Some(msg)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    /// Soft-delete all messages with id >= from_id for a session (rewind).
    pub fn rewind_messages(
        &self,
        session_id: &str,
        from_id: i64,
    ) -> Result<usize, AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "UPDATE agent_messages SET rewound_at = ?
             WHERE session_id = ? AND id >= ? AND rewound_at IS NULL",
            rusqlite::params![now_iso(), session_id, from_id],
        )?;
        Ok(conn.changes() as usize)
    }

    /// Soft-delete all messages with turn_count >= from_turn for a session.
    /// Used after a checkpoint restore so the conversation matches the files.
    pub fn rewind_from_turn(
        &self,
        session_id: &str,
        from_turn: u32,
    ) -> Result<usize, AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "UPDATE agent_messages SET rewound_at = ?
             WHERE session_id = ? AND turn_count >= ? AND rewound_at IS NULL",
            rusqlite::params![now_iso(), session_id, from_turn],
        )?;
        Ok(conn.changes() as usize)
    }

    // ── Context usage / reset ────────────────────────────────────────────

    /// Upsert token-usage stats for a session (one row per session).
    pub fn upsert_context_usage(
        &self,
        session_id: &str,
        project_path: &str,
        total_tokens: u32,
        context_limit: u32,
        message_count: u32,
    ) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        let metadata = serde_json::json!({
            "total_tokens": total_tokens,
            "context_limit": context_limit,
            "message_count": message_count,
        })
        .to_string();

        let tx = conn.unchecked_transaction()?;
        tx.execute(
            "DELETE FROM agent_messages WHERE session_id = ? AND type = 'context_usage'",
            [session_id],
        )?;
        tx.execute(
            "INSERT INTO agent_messages (session_id, project_path, role, type, llm_message, metadata, created_at)
             VALUES (?, ?, 'system', 'context_usage', '{}', ?, ?)",
            rusqlite::params![session_id, project_path, metadata, now_iso()],
        )?;
        tx.commit()?;
        Ok(())
    }

    /// Get persisted (total_tokens, context_limit, message_count) for a session.
    pub fn get_context_usage(
        &self,
        session_id: &str,
    ) -> Result<Option<(u32, u32, u32)>, AgentDbError> {
        let conn = self.conn.lock();
        let result = conn.query_row(
            "SELECT metadata FROM agent_messages
             WHERE session_id = ? AND type = 'context_usage' LIMIT 1",
            [session_id],
            |row| row.get::<_, String>(0),
        );
        match result {
            Ok(meta_str) => {
                let meta: serde_json::Value = serde_json::from_str(&meta_str).unwrap_or_default();
                Ok(Some((
                    meta["total_tokens"].as_u64().unwrap_or(0) as u32,
                    meta["context_limit"].as_u64().unwrap_or(0) as u32,
                    meta["message_count"].as_u64().unwrap_or(0) as u32,
                )))
            }
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    /// Delete the context_usage row for a session (after manual compaction).
    pub fn delete_context_usage(&self, session_id: &str) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "DELETE FROM agent_messages WHERE session_id = ? AND type = 'context_usage'",
            [session_id],
        )?;
        Ok(())
    }

    /// Insert a context_reset marker (hard floor for context loading) and clear usage.
    pub fn insert_context_reset(
        &self,
        session_id: &str,
        project_path: &str,
    ) -> Result<(), AgentDbError> {
        let conn = self.conn.lock();
        conn.execute(
            "INSERT INTO agent_messages (session_id, project_path, role, type, llm_message, metadata, created_at)
             VALUES (?, ?, 'system', 'context_reset', '{}', '{}', ?)",
            rusqlite::params![session_id, project_path, now_iso()],
        )?;
        conn.execute(
            "DELETE FROM agent_messages WHERE session_id = ? AND type = 'context_usage'",
            [session_id],
        )?;
        Ok(())
    }

    /// Insert a compaction record. Mirrors the agent loop's own persisted format.
    pub fn insert_compaction(
        &self,
        session_id: &str,
        project_path: &str,
        summary: &str,
        kept_before_count: u32,
    ) -> Result<String, AgentDbError> {
        let metadata = serde_json::json!({
            "version": 1,
            "kept_before_count": kept_before_count,
        })
        .to_string();
        let llm = serde_json::json!({"role": "system", "content": summary}).to_string();
        self.insert_message(session_id, project_path, "system", "compaction", &llm, &metadata, None)
    }
}

// ── Row mappers ──────────────────────────────────────────────────────────────

fn now_iso() -> String {
    chrono::Utc::now()
        .format("%Y-%m-%dT%H:%M:%S%.3fZ")
        .to_string()
}

fn map_session_row(row: &rusqlite::Row) -> rusqlite::Result<SessionRow> {
    Ok(SessionRow {
        id: row.get(0)?,
        folder: row.get(1)?,
        mode: row.get(2)?,
        title: row.get(3)?,
        parent_session_id: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
        status: row.get(7)?,
    })
}

/// Columns: id, session_id, project_path, role, type, llm_message, metadata,
/// created_at, rewound_at, turn_count (10 columns).
fn map_stored_message(row: &rusqlite::Row) -> rusqlite::Result<StoredMessage> {
    Ok(StoredMessage {
        id: row.get(0)?,
        session_id: row.get(1)?,
        project_path: row.get(2)?,
        role: row.get(3)?,
        type_: row.get(4)?,
        llm_message: row.get(5)?,
        metadata: row.get(6)?,
        created_at: row.get(7)?,
        rewound_at: row.get(8)?,
        turn_count: row.get(9)?,
    })
}

// ── Role/Type string conversions ───────────────────────────────────────────

fn role_to_str(role: MessageRole) -> &'static str {
    match role {
        MessageRole::User => "user",
        MessageRole::Assistant => "assistant",
        MessageRole::Tool => "tool",
        MessageRole::System => "system",
    }
}

fn type_to_str(mt: MessageType) -> &'static str {
    match mt {
        MessageType::Text => "text",
        MessageType::SessionInit => "session_init",
        MessageType::Compaction => "compaction",
        MessageType::ToolCall => "tool_call",
        MessageType::ToolResult => "tool_result",
        MessageType::CompletionSummary => "completion_summary",
    }
}

fn str_to_role(s: &str) -> MessageRole {
    persistence::str_to_role(s)
}

fn str_to_type(s: &str) -> MessageType {
    persistence::str_to_type(s)
}

// ── Context reconstruction ─────────────────────────────────────────────────

/// Reconstruct LLM context from stored messages, applying compaction.
/// Finds the last compaction record, reads `kept_before_count`, and keeps that
/// many non-compaction messages before it plus everything after.
pub fn reconstruct_context(stored: Vec<StoredMessage>) -> Vec<AgentMessage> {
    if stored.is_empty() {
        return Vec::new();
    }

    let last_compaction_pos = stored.iter().rposition(|m| m.type_ == "compaction");

    match last_compaction_pos {
        Some(pos) => {
            let kept_before_count = serde_json::from_str::<serde_json::Value>(&stored[pos].metadata)
                .ok()
                .and_then(|meta| meta["kept_before_count"].as_u64())
                .unwrap_or(0) as usize;

            let mut count = 0;
            let mut start = pos;
            while start > 0 && count < kept_before_count {
                start -= 1;
                if stored[start].type_ != "compaction" {
                    count += 1;
                }
            }

            stored
                .into_iter()
                .skip(start)
                .map(stored_to_agent_message)
                .collect()
        }
        None => stored.into_iter().map(stored_to_agent_message).collect(),
    }
}

fn stored_to_agent_message(stored: StoredMessage) -> AgentMessage {
    let llm_message: serde_json::Value =
        serde_json::from_str(&stored.llm_message).unwrap_or(serde_json::json!({}));
    let metadata: serde_json::Value =
        serde_json::from_str(&stored.metadata).unwrap_or(serde_json::json!({}));
    let content = llm_message["content"].as_str().unwrap_or("").to_string();
    let sender = if stored.role == "user" {
        Sender::HumanUser
    } else {
        Sender::Agent
    };

    AgentMessage {
        content,
        llm_message,
        metadata,
        role: str_to_role(&stored.role),
        message_type: str_to_type(&stored.type_),
        sender,
        turn_count: stored.turn_count,
    }
}

// ── SqliteMessagePersister ─────────────────────────────────────────────────

/// Implements [`MessagePersister`] backed by [`AgentDb`]. Messages are keyed by
/// the `session_id` passed to each call; `project_path` is stamped on every row
/// for reference. One instance is shared across a session and its subagents —
/// children persist under their own `session_id` (the crate stamps the parent
/// link into message metadata).
pub struct SqliteMessagePersister {
    db: Arc<AgentDb>,
    project_path: String,
}

impl SqliteMessagePersister {
    pub fn new(db: Arc<AgentDb>, project_path: String) -> Self {
        Self { db, project_path }
    }

    pub fn project_path(&self) -> &str {
        &self.project_path
    }
}

#[async_trait]
impl MessagePersister for SqliteMessagePersister {
    async fn persist_message(
        &self,
        message: &AgentMessage,
        session_id: &str,
    ) -> Result<PersistResult, PersistError> {
        let db = Arc::clone(&self.db);
        let sid = session_id.to_string();
        let project_path = self.project_path.clone();
        let role = role_to_str(message.role).to_string();
        let type_ = type_to_str(message.message_type).to_string();
        let turn_count = message.turn_count;
        let llm_message = serde_json::to_string(&message.llm_message)
            .map_err(|e| PersistError::Storage(e.to_string()))?;
        let metadata = serde_json::to_string(&message.metadata)
            .map_err(|e| PersistError::Storage(e.to_string()))?;

        let id = tokio::task::spawn_blocking(move || {
            db.insert_message(&sid, &project_path, &role, &type_, &llm_message, &metadata, turn_count)
        })
        .await
        .map_err(|e| PersistError::Storage(e.to_string()))?
        .map_err(|e| PersistError::Storage(e.to_string()))?;

        Ok(PersistResult { id })
    }

    async fn load_context(&self, session_id: &str) -> Result<Vec<AgentMessage>, PersistError> {
        let db = Arc::clone(&self.db);
        let sid = session_id.to_string();
        let stored = tokio::task::spawn_blocking(move || db.load_session_for_context(&sid, 500))
            .await
            .map_err(|e| PersistError::Storage(e.to_string()))?
            .map_err(|e| PersistError::Storage(e.to_string()))?;
        Ok(reconstruct_context(stored))
    }
}

// ── Tests ──────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use tempfile::tempdir;

    fn make_db() -> (tempfile::TempDir, AgentDb) {
        let dir = tempdir().unwrap();
        let db = AgentDb::new(dir.path()).unwrap();
        (dir, db)
    }

    fn make_persister() -> (tempfile::TempDir, Arc<AgentDb>, SqliteMessagePersister) {
        let dir = tempdir().unwrap();
        let db = Arc::new(AgentDb::new(dir.path()).unwrap());
        let persister = SqliteMessagePersister::new(Arc::clone(&db), "/proj".into());
        (dir, db, persister)
    }

    fn llm_json(role: &str, content: &str) -> String {
        json!({"role": role, "content": content}).to_string()
    }

    fn insert_text(db: &AgentDb, session_id: &str, content: &str) -> String {
        db.insert_message(session_id, "/proj", "user", "text", &llm_json("user", content), "{}", None)
            .unwrap()
    }

    fn agent_msg(content: &str, role: MessageRole, mt: MessageType) -> AgentMessage {
        AgentMessage {
            content: content.into(),
            llm_message: json!({"role": role_to_str(role), "content": content}),
            metadata: json!({}),
            role,
            message_type: mt,
            sender: if matches!(role, MessageRole::User) { Sender::HumanUser } else { Sender::Agent },
            turn_count: None,
        }
    }

    #[test]
    fn test_schema_v1() {
        let (_d, db) = make_db();
        assert_eq!(db.schema_version().unwrap(), 1);
    }

    #[test]
    fn test_session_crud() {
        let (_d, db) = make_db();
        db.create_session("s1", "/proj", "coding", Some("Fix bug"), None).unwrap();
        let s = db.get_session("s1").unwrap().unwrap();
        assert_eq!(s.folder, "/proj");
        assert_eq!(s.mode, "coding");
        assert_eq!(s.status, "active");

        // one-active-per-folder lookup
        assert_eq!(db.active_session_for_folder("/proj").unwrap().as_deref(), Some("s1"));
        db.set_session_status("s1", "idle").unwrap();
        assert!(db.active_session_for_folder("/proj").unwrap().is_none());

        let all = db.list_sessions().unwrap();
        assert_eq!(all.len(), 1);
    }

    #[test]
    fn test_session_isolation() {
        let (_d, db) = make_db();
        insert_text(&db, "a", "msg-a");
        insert_text(&db, "b", "msg-b");
        let a = db.load_session_messages("a").unwrap();
        assert_eq!(a.len(), 1);
        assert!(a[0].llm_message.contains("msg-a"));
    }

    #[tokio::test]
    async fn test_persister_roundtrip() {
        let (_d, _db, p) = make_persister();
        let msg = agent_msg("hello", MessageRole::Assistant, MessageType::Text);
        p.persist_message(&msg, "s1").await.unwrap();
        let loaded = p.load_context("s1").await.unwrap();
        assert_eq!(loaded.len(), 1);
        assert_eq!(loaded[0].content, "hello");
        assert_eq!(loaded[0].role, MessageRole::Assistant);
    }

    #[test]
    fn test_compaction_reconstruction() {
        let (_d, db) = make_db();
        for i in 1..=5 {
            insert_text(&db, "t1", &format!("msg-{i}"));
        }
        db.insert_compaction("t1", "/proj", "Summary of 1-3", 2).unwrap();
        for i in 6..=7 {
            insert_text(&db, "t1", &format!("msg-{i}"));
        }
        let stored = db.load_session_messages("t1").unwrap();
        let rec = reconstruct_context(stored);
        let contents: Vec<String> = rec.iter().map(|m| m.content.clone()).collect();
        for i in 1..=3 {
            assert!(!contents.contains(&format!("msg-{i}")), "msg-{i} should be compacted");
        }
        for i in 4..=7 {
            assert!(contents.contains(&format!("msg-{i}")), "msg-{i} should be present");
        }
        assert!(contents.iter().any(|c| c.contains("Summary of 1-3")));
    }

    #[test]
    fn test_context_usage_roundtrip() {
        let (_d, db) = make_db();
        db.upsert_context_usage("s1", "/proj", 1200, 128000, 8).unwrap();
        let usage = db.get_context_usage("s1").unwrap().unwrap();
        assert_eq!(usage, (1200, 128000, 8));
        db.delete_context_usage("s1").unwrap();
        assert!(db.get_context_usage("s1").unwrap().is_none());
    }

    #[test]
    fn test_rewind() {
        let (_d, db) = make_db();
        let _id1 = insert_text(&db, "t1", "keep");
        let id2: i64 = insert_text(&db, "t1", "drop").parse().unwrap();
        insert_text(&db, "t1", "also drop");
        let n = db.rewind_messages("t1", id2).unwrap();
        assert_eq!(n, 2);
        let remaining = db.load_session_messages("t1").unwrap();
        assert_eq!(remaining.len(), 1);
        assert!(remaining[0].llm_message.contains("keep"));
    }
}
