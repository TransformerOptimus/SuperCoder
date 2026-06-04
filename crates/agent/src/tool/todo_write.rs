use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::types::{AgentEvent, TodoItem};
use super::{Tool, ToolContext, ToolResult};

/// Tool that lets the agent track progress on multi-step tasks.
///
/// Each call replaces the entire todo list (replace-all semantics).
/// The list is written to `{working_dir}/.agent/todos.md` for persistence
/// across context compaction, and emitted as a `TodoUpdated` event for the UI.
pub struct TodoWriteTool;

#[async_trait]
impl Tool for TodoWriteTool {
    fn name(&self) -> &str {
        "todo_write"
    }

    fn description(&self) -> &str {
        "Write or update a todo list to track progress on multi-step tasks. \
         Each call replaces the entire list. Mark items as completed as you finish them."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["todos"],
            "properties": {
                "todos": {
                    "type": "array",
                    "description": "The complete todo list. Each call replaces the entire list.",
                    "items": {
                        "type": "object",
                        "required": ["id", "content", "status"],
                        "properties": {
                            "id": {
                                "type": "string",
                                "description": "Unique identifier for this todo item"
                            },
                            "content": {
                                "type": "string",
                                "description": "Description of the task"
                            },
                            "status": {
                                "type": "string",
                                "enum": ["pending", "in_progress", "completed"],
                                "description": "Current status of the task"
                            }
                        }
                    }
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let todos_value = match args.get("todos") {
            Some(v) if v.is_array() => v,
            _ => return Ok(ToolResult::error("'todos' must be a non-empty array.")),
        };

        let arr = todos_value.as_array().unwrap();
        let mut todos: Vec<TodoItem> = Vec::with_capacity(arr.len());

        for (i, item) in arr.iter().enumerate() {
            let id = item
                .get("id")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let content = item
                .get("content")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let status = item
                .get("status")
                .and_then(|v| v.as_str())
                .unwrap_or("pending")
                .to_string();

            if id.is_empty() || content.is_empty() {
                return Ok(ToolResult::error(format!(
                    "Todo item at index {} must have non-empty 'id' and 'content'.",
                    i
                )));
            }

            match status.as_str() {
                "pending" | "in_progress" | "completed" => {}
                _ => {
                    return Ok(ToolResult::error(format!(
                        "Todo item '{}' has invalid status '{}'. Must be: pending, in_progress, or completed.",
                        id, status
                    )));
                }
            }

            todos.push(TodoItem { id, content, status });
        }

        let status_counts: (usize, usize, usize) = todos.iter().fold((0,0,0), |mut acc, t| {
            match t.status.as_str() { "pending" => acc.0 += 1, "in_progress" => acc.1 += 1, "completed" => acc.2 += 1, _ => {} }; acc
        });
        log::info!("[v1.0] todo_write: {} items (pending={}, in_progress={}, completed={})", todos.len(), status_counts.0, status_counts.1, status_counts.2);

        // Format as markdown
        let mut md = String::from("# Agent Todos\n\n");
        for todo in &todos {
            let marker = match todo.status.as_str() {
                "completed" => "[x]",
                "in_progress" => "[-]",
                _ => "[ ]",
            };
            md.push_str(&format!("- {} {}\n", marker, todo.content));
        }

        // Write to .agent/todos.md
        let agent_dir = crate::util::ensure_agent_dir(&ctx.working_dir).await;

        let todo_path = agent_dir.join("todos.md");
        if let Err(e) = tokio::fs::write(&todo_path, &md).await {
            log::warn!("Failed to write todos.md: {e}");
        }

        // Emit event for UI
        let _ = ctx.event_tx.send(AgentEvent::TodoUpdated {
            session_id: ctx.session_id.clone(),
            todos: todos.clone(),
        }).await;

        // Return the formatted list as tool output
        Ok(ToolResult::success(md))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_ctx(dir: &std::path::Path) -> ToolContext {
        ToolContext::test_context(dir)
    }

    #[tokio::test]
    async fn test_todo_write_basic() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = TodoWriteTool;
        let result = tool
            .execute(
                json!({
                    "todos": [
                        {"id": "1", "content": "Read the file", "status": "completed"},
                        {"id": "2", "content": "Edit the function", "status": "in_progress"},
                        {"id": "3", "content": "Run tests", "status": "pending"}
                    ]
                }),
                &test_ctx(tmp.path()),
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("[x] Read the file"));
        assert!(result.output.contains("[-] Edit the function"));
        assert!(result.output.contains("[ ] Run tests"));
    }

    #[tokio::test]
    async fn test_todo_write_creates_file() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = TodoWriteTool;
        tool.execute(
            json!({
                "todos": [
                    {"id": "1", "content": "Task one", "status": "pending"}
                ]
            }),
            &test_ctx(tmp.path()),
        )
        .await
        .unwrap();

        let todo_path = tmp.path().join(".agent").join("todos.md");
        assert!(todo_path.exists());
        let content = tokio::fs::read_to_string(&todo_path).await.unwrap();
        assert!(content.contains("Task one"));
    }

    #[tokio::test]
    async fn test_todo_write_replaces_on_update() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = TodoWriteTool;

        // First write
        tool.execute(
            json!({"todos": [{"id": "1", "content": "First task", "status": "pending"}]}),
            &test_ctx(tmp.path()),
        )
        .await
        .unwrap();

        // Second write — replaces
        tool.execute(
            json!({"todos": [{"id": "2", "content": "Second task", "status": "completed"}]}),
            &test_ctx(tmp.path()),
        )
        .await
        .unwrap();

        let content = tokio::fs::read_to_string(tmp.path().join(".agent/todos.md"))
            .await
            .unwrap();
        assert!(!content.contains("First task"));
        assert!(content.contains("Second task"));
    }

    #[tokio::test]
    async fn test_todo_write_empty_list() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = TodoWriteTool;
        let result = tool
            .execute(json!({"todos": []}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("# Agent Todos"));
    }

    #[tokio::test]
    async fn test_todo_write_invalid_status() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = TodoWriteTool;
        let result = tool
            .execute(
                json!({"todos": [{"id": "1", "content": "Task", "status": "done"}]}),
                &test_ctx(tmp.path()),
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("invalid status"));
    }

    #[tokio::test]
    async fn test_todo_write_missing_content() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = TodoWriteTool;
        let result = tool
            .execute(
                json!({"todos": [{"id": "1", "content": "", "status": "pending"}]}),
                &test_ctx(tmp.path()),
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("non-empty"));
    }
}
