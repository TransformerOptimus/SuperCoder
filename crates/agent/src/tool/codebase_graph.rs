use std::sync::Arc;

use async_trait::async_trait;
use serde_json::{json, Value};

use crate::context_engine::ContextEngineApi;
use crate::error::ToolError;
use super::{Tool, ToolContext, ToolResult};

pub struct CodebaseGraphTool {
    context_engine: Arc<dyn ContextEngineApi>,
}

impl CodebaseGraphTool {
    pub fn new(context_engine: Arc<dyn ContextEngineApi>) -> Self {
        Self { context_engine }
    }
}

#[async_trait]
impl Tool for CodebaseGraphTool {
    fn name(&self) -> &str {
        "codebase_graph"
    }

    fn description(&self) -> &str {
        "Get the dependency graph for a function or symbol. Returns who calls it \
         (blast radius / upstream) and what it calls (dependencies / downstream). \
         USE THIS BEFORE modifying a function to understand what code depends on it and what \
         might break. Essential for refactoring, deprecations, and impact analysis. \
         Requires at least one of: query, function_name, or file_path. \
         If the graph is still being built or unavailable, this tool will tell you — fall back \
         to grep/glob for textual searches in that case."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Natural language query about code dependencies (e.g., 'what calls the auth middleware')"
                },
                "function_name": {
                    "type": "string",
                    "description": "Specific function/method name to look up directly in the graph"
                },
                "file_path": {
                    "type": "string",
                    "description": "File path to disambiguate function lookup when name is not unique"
                },
                "query_type": {
                    "type": "string",
                    "enum": ["blast_radius", "dependencies"],
                    "description": "Type of graph query. 'blast_radius' finds upstream callers. 'dependencies' finds downstream callees. Omit for both."
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let query = args.get("query").and_then(|v| v.as_str());
        let function_name = args.get("function_name").and_then(|v| v.as_str());
        let file_path = args.get("file_path").and_then(|v| v.as_str());
        let query_type = args.get("query_type").and_then(|v| v.as_str());

        // Validate at least one lookup key is provided
        if query.is_none() && function_name.is_none() {
            return Ok(ToolResult::error(
                "Either 'query' or 'function_name' must be provided. Use 'query' for \
                 natural language ('what calls the auth handler') or 'function_name' for \
                 direct lookup ('ValidateToken')."
            ));
        }

        let _ = ctx; // ToolContext not needed for graph queries (no working-copy overlay)

        log::info!(
            "[codebase_graph] query={:?}, function_name={:?}, file_path={:?}, query_type={:?}",
            query, function_name, file_path, query_type
        );

        let start = std::time::Instant::now();
        let response = match self
            .context_engine
            .graph_query(query, function_name, file_path, query_type)
            .await
        {
            Ok(resp) => resp,
            Err(e) => {
                log::warn!("[codebase_graph] Context engine error after {:?}: {e}", start.elapsed());
                return Ok(ToolResult::error(format!(
                    "Context engine is unavailable. Use grep to find function references \
                     manually. Error: {e}"
                )));
            }
        };
        let elapsed = start.elapsed();

        if response.indexing {
            log::info!("[codebase_graph] Graph not ready (took {:?})", elapsed);
            return Ok(ToolResult::success(
                "The codebase dependency graph is being built. Use grep to find function \
                 references manually while indexing completes."
            ));
        }

        if response.results.is_empty() {
            log::info!("[codebase_graph] 0 results (took {:?})", elapsed);
            return Ok(ToolResult::success(
                "No dependency information found for this query. The function may not be \
                 indexed, or it has no callers/dependencies in the graph."
            ));
        }

        let callers_count = response.results.iter().filter(|r| r.direction.as_deref() == Some("caller")).count();
        let deps_count = response.results.iter().filter(|r| r.direction.as_deref() == Some("dependency")).count();
        log::info!(
            "[codebase_graph] {} results (callers={}, deps={}, took {:?}). Top: {}",
            response.results.len(),
            callers_count,
            deps_count,
            elapsed,
            response.results.iter().take(5).map(|r| format!("{}({})", r.name, r.file_path)).collect::<Vec<_>>().join(", ")
        );

        // Build the lookup label for the header
        let lookup_label = if let Some(fname) = function_name {
            fname.to_string()
        } else if let Some(q) = query {
            q.to_string()
        } else {
            "query".to_string()
        };

        // Partition results by direction
        let callers: Vec<_> = response
            .results
            .iter()
            .filter(|r| r.direction.as_deref() == Some("caller"))
            .collect();
        let dependencies: Vec<_> = response
            .results
            .iter()
            .filter(|r| r.direction.as_deref() == Some("dependency"))
            .collect();
        // Results with no direction or unknown direction
        let other: Vec<_> = response
            .results
            .iter()
            .filter(|r| {
                r.direction.as_deref() != Some("caller")
                    && r.direction.as_deref() != Some("dependency")
            })
            .collect();

        let mut output = format!(
            "Dependency graph for \"{}\" — {} results:\n\n",
            lookup_label,
            response.results.len()
        );

        let show_callers = query_type.is_none() || query_type == Some("blast_radius");
        let show_deps = query_type.is_none() || query_type == Some("dependencies");

        if show_callers && !callers.is_empty() {
            output.push_str("## Blast Radius (upstream callers)\n\n");
            for (i, item) in callers.iter().enumerate() {
                output.push_str(&format!(
                    "{}. {} ({}, depth: {})\n",
                    i + 1,
                    item.name,
                    item.file_path,
                    item.depth,
                ));
            }
            output.push('\n');
        }

        if show_deps && !dependencies.is_empty() {
            output.push_str("## Dependencies (downstream)\n\n");
            for (i, item) in dependencies.iter().enumerate() {
                output.push_str(&format!(
                    "{}. {} ({}, depth: {})\n",
                    i + 1,
                    item.name,
                    item.file_path,
                    item.depth,
                ));
            }
            output.push('\n');
        }

        if !other.is_empty() && query_type.is_none() {
            output.push_str("## Related\n\n");
            for (i, item) in other.iter().enumerate() {
                output.push_str(&format!(
                    "{}. {} ({}, depth: {})\n",
                    i + 1,
                    item.name,
                    item.file_path,
                    item.depth,
                ));
            }
            output.push('\n');
        }

        Ok(ToolResult::success(output))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::context_engine::{
        ContextEngineError, GraphQueryResponse, GraphResultItem, IndexStatusResponse,
        MockContextEngine, SearchResponse,
    };
    use crate::tool::ToolContext;

    // --- Test-only mock that always returns errors ---
    struct FailingContextEngine;

    #[async_trait]
    impl ContextEngineApi for FailingContextEngine {
        async fn index_status(&self) -> Result<IndexStatusResponse, ContextEngineError> {
            Err(ContextEngineError::Unavailable("test error".into()))
        }
        async fn search(
            &self,
            _query: &str,
            _strategy: Option<&str>,
            _limit: Option<u32>,
        ) -> Result<SearchResponse, ContextEngineError> {
            Err(ContextEngineError::Unavailable("test error".into()))
        }
        async fn graph_query(
            &self,
            _query: Option<&str>,
            _function_name: Option<&str>,
            _file_path: Option<&str>,
            _query_type: Option<&str>,
        ) -> Result<GraphQueryResponse, ContextEngineError> {
            Err(ContextEngineError::Unavailable("test error".into()))
        }
    }

    fn make_caller_results() -> Vec<GraphResultItem> {
        vec![
            GraphResultItem {
                name: "AuthMiddleware".into(),
                file_path: "src/handler.go".into(),
                chunk_id: "g1".into(),
                depth: 1,
                direction: Some("caller".into()),
            },
            GraphResultItem {
                name: "HandleWebSocket".into(),
                file_path: "src/ws/connection.go".into(),
                chunk_id: "g2".into(),
                depth: 2,
                direction: Some("caller".into()),
            },
        ]
    }

    fn make_dependency_results() -> Vec<GraphResultItem> {
        vec![GraphResultItem {
            name: "jwt.ParseWithClaims".into(),
            file_path: "vendor/jwt/parser.go".into(),
            chunk_id: "g3".into(),
            depth: 1,
            direction: Some("dependency".into()),
        }]
    }

    fn make_mixed_results() -> Vec<GraphResultItem> {
        let mut results = make_caller_results();
        results.extend(make_dependency_results());
        results
    }

    #[tokio::test]
    async fn test_graph_formats_blast_radius() {
        let mock = MockContextEngine::with_graph_results(make_caller_results());
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"function_name": "ValidateToken", "query_type": "blast_radius"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Blast Radius"));
        assert!(result.output.contains("AuthMiddleware"));
        assert!(result.output.contains("HandleWebSocket"));
        assert!(result.output.contains("src/handler.go"));
        assert!(result.output.contains("depth: 1"));
        assert!(result.output.contains("depth: 2"));
    }

    #[tokio::test]
    async fn test_graph_formats_dependencies() {
        let mock = MockContextEngine::with_graph_results(make_dependency_results());
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"function_name": "ValidateToken", "query_type": "dependencies"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Dependencies"));
        assert!(result.output.contains("jwt.ParseWithClaims"));
        assert!(result.output.contains("vendor/jwt/parser.go"));
    }

    #[tokio::test]
    async fn test_graph_formats_mixed() {
        let mock = MockContextEngine::with_graph_results(make_mixed_results());
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"function_name": "ValidateToken"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Blast Radius"));
        assert!(result.output.contains("Dependencies"));
        assert!(result.output.contains("3 results"));
    }

    #[tokio::test]
    async fn test_graph_cold_start() {
        let mock = MockContextEngine::not_indexed();
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"function_name": "anything"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("graph is being built"));
        assert!(result.output.contains("grep"));
    }

    #[tokio::test]
    async fn test_graph_unavailable() {
        let tool = CodebaseGraphTool::new(Arc::new(FailingContextEngine));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"function_name": "anything"}), &ctx)
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("unavailable"));
        assert!(result.output.contains("grep"));
    }

    #[tokio::test]
    async fn test_graph_empty() {
        let mock = MockContextEngine::indexed_empty();
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"function_name": "NonexistentFn"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("No dependency information"));
    }

    #[tokio::test]
    async fn test_graph_no_params() {
        let mock = MockContextEngine::indexed_empty();
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool.execute(json!({}), &ctx).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("'query' or 'function_name'"));
    }

    #[tokio::test]
    async fn test_graph_function_name_only() {
        let mock = MockContextEngine::with_graph_results(make_caller_results());
        let tool = CodebaseGraphTool::new(Arc::new(mock));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"function_name": "ValidateToken"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("ValidateToken"));
    }
}
