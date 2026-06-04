use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use super::{Tool, ToolContext, ToolResult};

/// Tool that lets the agent ask the user a clarifying question.
///
/// Uses the yield pattern (same as save_plan): sets `yield_data` on the result,
/// which causes the agent loop to yield `AgentResult::AskUser`. The caller collects
/// the user's answer and re-invokes `run()` with the answer as a new user message.
pub struct AskUserTool;

#[async_trait]
impl Tool for AskUserTool {
    fn name(&self) -> &str {
        "ask_user"
    }

    fn description(&self) -> &str {
        "Ask the user a clarifying question. Use when you need more information to proceed. \
         Optionally provide a list of choices for the user to pick from."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["question"],
            "properties": {
                "question": {
                    "type": "string",
                    "description": "The question to ask the user"
                },
                "options": {
                    "type": "array",
                    "items": { "type": "string" },
                    "description": "Optional list of choices for the user to pick from"
                }
            }
        })
    }

    async fn execute(&self, args: Value, _ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let question = args
            .get("question")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();

        if question.is_empty() {
            return Ok(ToolResult::error("Question must not be empty."));
        }

        let options: Option<Vec<String>> = args.get("options").and_then(|v| {
            v.as_array().map(|arr| {
                arr.iter()
                    .filter_map(|item| item.as_str().map(String::from))
                    .collect()
            })
        });

        let yield_data = json!({
            "yield_type": "ask_user",
            "question": question,
            "options": options,
        });

        Ok(ToolResult {
            output: format!("Question asked: {question}"),
            is_error: false,
            yield_data: Some(yield_data),
            modified_files: Vec::new(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_ctx() -> ToolContext {
        ToolContext::test_context(std::path::Path::new("/tmp"))
    }

    #[tokio::test]
    async fn test_ask_user_basic() {
        let tool = AskUserTool;
        let result = tool
            .execute(json!({"question": "Which database should we use?"}), &test_ctx())
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Which database should we use?"));
        assert!(result.yield_data.is_some());

        let data = result.yield_data.unwrap();
        assert_eq!(data["yield_type"], "ask_user");
        assert_eq!(data["question"], "Which database should we use?");
        assert!(data["options"].is_null());
    }

    #[tokio::test]
    async fn test_ask_user_with_options() {
        let tool = AskUserTool;
        let result = tool
            .execute(
                json!({
                    "question": "Which approach?",
                    "options": ["Option A", "Option B", "Option C"]
                }),
                &test_ctx(),
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        let data = result.yield_data.unwrap();
        assert_eq!(data["yield_type"], "ask_user");
        let opts = data["options"].as_array().unwrap();
        assert_eq!(opts.len(), 3);
        assert_eq!(opts[0], "Option A");
    }

    #[tokio::test]
    async fn test_ask_user_empty_question() {
        let tool = AskUserTool;
        let result = tool
            .execute(json!({"question": ""}), &test_ctx())
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.yield_data.is_none());
    }

    #[tokio::test]
    async fn test_ask_user_missing_question() {
        let tool = AskUserTool;
        let result = tool.execute(json!({}), &test_ctx()).await.unwrap();

        assert!(result.is_error);
        assert!(result.yield_data.is_none());
    }
}
