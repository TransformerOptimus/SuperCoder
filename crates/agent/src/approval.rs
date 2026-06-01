use async_trait::async_trait;
use serde_json::Value;

/// Outcome of an approval request.
#[derive(Debug, Clone, PartialEq)]
pub enum ApprovalDecision {
    /// User approved the tool execution.
    Approved,
    /// User denied the tool execution.
    Denied { reason: Option<String> },
}

/// Trait for requesting user approval before tool execution.
/// Implementors should emit a UI event and block until the user responds.
///
/// When `None` is passed as the handler, all tools are auto-approved (current behavior).
#[async_trait]
pub trait ApprovalHandler: Send + Sync {
    /// Request approval for a specific tool call.
    /// Returns the user's decision. Blocks until the user responds or the session is cancelled.
    async fn request_approval(
        &self,
        tool_name: &str,
        tool_call_id: &str,
        args: &Value,
        args_summary: &str,
    ) -> ApprovalDecision;
}

#[cfg(test)]
pub mod test_util {
    use super::*;
    use std::sync::{Arc, Mutex};

    /// Mock handler that auto-approves everything.
    pub struct AutoApproveHandler;

    #[async_trait]
    impl ApprovalHandler for AutoApproveHandler {
        async fn request_approval(
            &self, _: &str, _: &str, _: &Value, _: &str,
        ) -> ApprovalDecision {
            ApprovalDecision::Approved
        }
    }

    /// Mock handler that denies everything.
    pub struct AutoDenyHandler {
        pub reason: String,
    }

    #[async_trait]
    impl ApprovalHandler for AutoDenyHandler {
        async fn request_approval(
            &self, _: &str, _: &str, _: &Value, _: &str,
        ) -> ApprovalDecision {
            ApprovalDecision::Denied {
                reason: Some(self.reason.clone()),
            }
        }
    }

    /// Mock handler that records calls and returns queued decisions.
    pub struct RecordingApprovalHandler {
        pub calls: Arc<Mutex<Vec<String>>>,
        decisions: Arc<Mutex<Vec<ApprovalDecision>>>,
    }

    impl RecordingApprovalHandler {
        pub fn new() -> Self {
            Self {
                calls: Arc::new(Mutex::new(Vec::new())),
                decisions: Arc::new(Mutex::new(Vec::new())),
            }
        }

        pub fn queue_decision(&self, decision: ApprovalDecision) {
            self.decisions.lock().unwrap().push(decision);
        }
    }

    #[async_trait]
    impl ApprovalHandler for RecordingApprovalHandler {
        async fn request_approval(
            &self, tool_name: &str, tool_call_id: &str, _: &Value, _: &str,
        ) -> ApprovalDecision {
            self.calls
                .lock()
                .unwrap()
                .push(format!("{tool_name}:{tool_call_id}"));
            let mut decisions = self.decisions.lock().unwrap();
            assert!(
                !decisions.is_empty(),
                "RecordingApprovalHandler: no decisions queued for {tool_name}:{tool_call_id}"
            );
            decisions.remove(0)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use super::test_util::*;
    use serde_json::json;

    #[tokio::test]
    async fn test_auto_approve_handler_approves_everything() {
        let handler = AutoApproveHandler;
        let decision = handler
            .request_approval("bash", "call-1", &json!({}), "Running command")
            .await;
        assert_eq!(decision, ApprovalDecision::Approved);
    }

    #[tokio::test]
    async fn test_auto_deny_handler_denies_everything() {
        let handler = AutoDenyHandler {
            reason: "not allowed".into(),
        };
        let decision = handler
            .request_approval("read", "call-2", &json!({}), "Reading file")
            .await;
        assert_eq!(
            decision,
            ApprovalDecision::Denied {
                reason: Some("not allowed".into())
            }
        );
    }

    #[tokio::test]
    async fn test_recording_handler_records_calls() {
        let handler = RecordingApprovalHandler::new();
        handler.queue_decision(ApprovalDecision::Approved);

        let decision = handler
            .request_approval("edit", "call-1", &json!({}), "Editing file")
            .await;

        assert_eq!(decision, ApprovalDecision::Approved);
        let calls = handler.calls.lock().unwrap();
        assert_eq!(*calls, vec!["edit:call-1"]);
    }
}
