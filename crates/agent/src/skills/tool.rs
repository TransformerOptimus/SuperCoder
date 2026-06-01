use std::sync::Arc;

use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::tool::{Tool, ToolContext, ToolResult};
use crate::types::AgentEvent;

use super::registry::SkillRegistry;

/// Tool that loads a skill body into the conversation on demand.
pub struct SkillTool {
    registry: Arc<SkillRegistry>,
}

impl SkillTool {
    pub fn new(registry: Arc<SkillRegistry>) -> Self {
        Self { registry }
    }
}

#[async_trait]
impl Tool for SkillTool {
    fn name(&self) -> &str {
        "skill"
    }

    fn description(&self) -> &str {
        "Load a skill's full instructions into the conversation. The skills list at the top of the \
         system prompt shows available names and descriptions. Call this when a task matches a \
         skill and you need the full body to follow its instructions."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["name"],
            "properties": {
                "name": {
                    "type": "string",
                    "description": "The exact skill name from the Available Skills list."
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let name = args
            .get("name")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: name".into()))?;

        log::info!("[skills] SkillTool::execute requested name={:?}", name);

        let Some(skill) = self.registry.get(name) else {
            let available = self.registry.names().join(", ");
            log::warn!(
                "[skills] SkillTool::execute unknown name={:?}, available=[{}]",
                name,
                available
            );
            return Ok(ToolResult::error(format!(
                "Unknown skill `{name}`. Available skills: [{available}]"
            )));
        };

        log::info!(
            "[skills] SkillTool::execute HIT name={} body_len={} origin={:?} — emitting SkillLoaded",
            skill.name,
            skill.body.len(),
            skill.origin
        );

        let _ = ctx
            .event_tx
            .send(AgentEvent::SkillLoaded {
                session_id: ctx.session_id.clone(),
                skill_name: skill.name.clone(),
            })
            .await;

        // Escape any literal </skill_content> in the body so it doesn't prematurely
        // close the wrapper tag when the LLM parses the tool result. Accidental
        // collisions happen when a skill documents the skill system itself.
        //
        // TODO(skills): case-sensitive exact match — variants like `</Skill_Content>`
        // or `< /skill_content>` slip through. Low risk given the tag name is
        // deliberately obscure, but worth revisiting if a meta-skill trips on it.
        let escaped_body = skill.body.replace("</skill_content>", "<\\/skill_content>");

        let output = format!(
            "<skill_content name=\"{}\">\n{}\n</skill_content>",
            skill.name, escaped_body
        );
        Ok(ToolResult::success(output))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::skills::registry::SkillInput;
    use std::collections::HashSet;
    use std::path::PathBuf;
    use tokio::sync::mpsc;
    use tokio_util::sync::CancellationToken;

    fn mk_registry() -> Arc<SkillRegistry> {
        let s = SkillInput {
            raw: "---\nname: hello\ndescription: A greeting skill.\n---\nBe a pirate.\n"
                .to_string(),
            path: PathBuf::from("/hello"),
        };
        Arc::new(SkillRegistry::new(
            vec![],
            vec![s],
            vec![],
            &HashSet::new(),
        ))
    }

    #[tokio::test]
    async fn returns_error_for_unknown_name() {
        let tool = SkillTool::new(mk_registry());
        let (tx, _rx) = mpsc::channel(8);
        let ctx = ToolContext {
            working_dir: PathBuf::from("."),
            cancel_token: CancellationToken::new(),
            event_tx: tx,
            session_id: "sess".into(),
            tool_call_id: "tc_1".into(),
        };
        let result = tool
            .execute(json!({"name": "nope"}), &ctx)
            .await
            .expect("execute ok");
        assert!(result.is_error);
        assert!(result.output.contains("Unknown skill"));
        assert!(result.output.contains("hello"));
    }

    #[tokio::test]
    async fn loads_body_and_emits_event() {
        let tool = SkillTool::new(mk_registry());
        let (tx, mut rx) = mpsc::channel(8);
        let ctx = ToolContext {
            working_dir: PathBuf::from("."),
            cancel_token: CancellationToken::new(),
            event_tx: tx,
            session_id: "sess".into(),
            tool_call_id: "tc_1".into(),
        };
        let result = tool
            .execute(json!({"name": "hello"}), &ctx)
            .await
            .expect("execute ok");
        assert!(!result.is_error);
        assert!(result.output.contains("<skill_content name=\"hello\">"));
        assert!(result.output.contains("Be a pirate."));

        let event = rx.recv().await.expect("event emitted");
        match event {
            AgentEvent::SkillLoaded { session_id, skill_name } => {
                assert_eq!(session_id, "sess");
                assert_eq!(skill_name, "hello");
            }
            other => panic!("expected SkillLoaded, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn escapes_close_tag_in_body() {
        let s = SkillInput {
            raw: "---\nname: meta\ndescription: A skill that mentions </skill_content> in its body.\n---\nWhen you see </skill_content> in code, leave it alone.\n".to_string(),
            path: PathBuf::from("/meta"),
        };
        let registry = Arc::new(SkillRegistry::new(
            vec![],
            vec![s],
            vec![],
            &HashSet::new(),
        ));
        let tool = SkillTool::new(registry);
        let (tx, _rx) = mpsc::channel(8);
        let ctx = ToolContext {
            working_dir: PathBuf::from("."),
            cancel_token: CancellationToken::new(),
            event_tx: tx,
            session_id: "sess".into(),
            tool_call_id: "tc_1".into(),
        };
        let result = tool
            .execute(json!({"name": "meta"}), &ctx)
            .await
            .expect("execute ok");
        assert!(!result.is_error);
        // The wrapping </skill_content> appears exactly once, at the end.
        assert_eq!(result.output.matches("</skill_content>").count(), 1);
        // The body's original literal was escaped.
        assert!(result.output.contains("<\\/skill_content>"));
    }

    #[tokio::test]
    async fn rejects_missing_name_arg() {
        let tool = SkillTool::new(mk_registry());
        let (tx, _rx) = mpsc::channel(8);
        let ctx = ToolContext {
            working_dir: PathBuf::from("."),
            cancel_token: CancellationToken::new(),
            event_tx: tx,
            session_id: "sess".into(),
            tool_call_id: "tc_1".into(),
        };
        let err = tool.execute(json!({}), &ctx).await.unwrap_err();
        assert!(err.0.contains("Missing"));
    }
}
