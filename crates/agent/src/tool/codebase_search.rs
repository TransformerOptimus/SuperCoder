use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio::process::Command;

use crate::context_engine::{ContextEngineApi, SearchResultItem};
use crate::error::ToolError;
use super::{Tool, ToolContext, ToolResult};

pub struct CodebaseSearchTool {
    context_engine: Arc<dyn ContextEngineApi>,
    /// Canonical repo path the context-engine index was built from (for logging /
    /// index identity). The agent edits this same dir in place.
    repo_path: PathBuf,
}

impl CodebaseSearchTool {
    pub fn new(context_engine: Arc<dyn ContextEngineApi>, repo_path: PathBuf) -> Self {
        Self { context_engine, repo_path }
    }
}

#[async_trait]
impl Tool for CodebaseSearchTool {
    fn name(&self) -> &str {
        "codebase_search"
    }

    fn description(&self) -> &str {
        "Semantic code search across the entire indexed codebase. Returns relevant code chunks \
         ranked by AI-computed relevance (not keyword match). \
         USE THIS FIRST when you need to: understand how a feature works, find where something \
         is implemented, or explore an unfamiliar codebase. Handles conceptual queries like \
         'authentication flow' or 'rate limiting logic'. Much more powerful than grep for \
         anything that isn't an exact string match. \
         If the index is still being built or unavailable, this tool will tell you — fall back \
         to grep/glob in that case."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["query"],
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Natural language description of what you're looking for, or a code identifier / function name"
                },
                "strategy": {
                    "type": "string",
                    "enum": ["multi", "vector", "keyword", "graph", "hybrid"],
                    "description": "Search strategy. 'multi' (default) combines vector+keyword. 'vector' for semantic/conceptual. 'keyword' for exact identifiers. 'graph' for dependency-aware. 'hybrid' for vector+graph blend."
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of results to return (default: 20)"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let query = args
            .get("query")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: query".into()))?;

        let strategy = args.get("strategy").and_then(|v| v.as_str());
        let limit = args.get("limit").and_then(|v| v.as_u64().map(|n| n as u32));

        log::info!(
            "[codebase_search] query=\"{}\", strategy={:?}, limit={:?}",
            query, strategy, limit
        );

        let start = std::time::Instant::now();
        let response = match self.context_engine.search(query, strategy, limit).await {
            Ok(resp) => resp,
            Err(e) => {
                log::warn!("[codebase_search] Context engine error after {:?}: {e}", start.elapsed());
                return Ok(ToolResult::error(format!(
                    "Context engine is unavailable. Use grep and glob tools to search the \
                     codebase directly. Error: {e}"
                )));
            }
        };
        let elapsed = start.elapsed();

        if response.indexing {
            log::info!("[codebase_search] Index not ready (took {:?})", elapsed);
            return Ok(ToolResult::success(
                "The codebase is currently being indexed. This is a one-time process that \
                 takes 2-5 minutes. Use grep and glob tools to search manually while \
                 indexing completes."
            ));
        }

        if response.results.is_empty() {
            log::info!("[codebase_search] 0 results for \"{}\" (took {:?})", query, elapsed);
            return Ok(ToolResult::success(format!(
                "No results found for query '{query}'. Try rephrasing, using a different \
                 strategy (keyword for exact matches, vector for conceptual), or use grep \
                 for exact pattern matching."
            )));
        }

        log::info!(
            "[codebase_search] {} results for \"{}\" (took {:?}). Top files: {}",
            response.results.len(),
            query,
            elapsed,
            response.results.iter().take(5).map(|r| format!("{}({:.2})", r.file_path, r.score)).collect::<Vec<_>>().join(", ")
        );

        let mut results = response.results;

        // Reconcile results against the user's live working copy. The context-engine
        // index is built from the canonical repo (`self.repo_path`) and lags the
        // agent's in-place edits, so we always overlay `git diff HEAD` from the
        // working dir to drop deleted files and flag stale (modified) ones.
        log::info!(
            "[codebase_search] applying local overlay (working_dir={:?}, index_repo={:?})",
            ctx.working_dir, self.repo_path
        );
        if let Err(e) = apply_local_overlay(&mut results, &ctx.working_dir).await {
            log::warn!("[codebase_search] local overlay failed, skipping: {e}");
        }

        let mut output = format!("Found {} results for \"{}\":\n\n", results.len(), query);
        for (i, item) in results.iter().enumerate() {
            output.push_str(&format!(
                "## {}. {} (score: {:.2}, source: {})\n```{}\n{}\n```\n\n",
                i + 1,
                item.file_path,
                item.score,
                item.source,
                item.language,
                item.content,
            ));
        }

        Ok(ToolResult::success(output))
    }
}

/// Overlay search results with the live working-copy state.
/// Modified files: annotated (chunk content may be stale; no line-range data to re-extract).
/// Deleted files: drop from results.
/// New files: NOT added (not in index to match against).
async fn apply_local_overlay(
    results: &mut Vec<SearchResultItem>,
    working_dir: &Path,
) -> Result<(), ToolError> {
    // 1. Get all modified + added files
    let mut cmd = Command::new("git");
    cmd.args(["diff", "--name-only", "HEAD"])
        .current_dir(working_dir);
    git_ops::no_window::no_window_tokio(&mut cmd);
    let output = cmd
        .output()
        .await
        .map_err(|e| ToolError(format!("git diff failed: {e}")))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(ToolError(format!("git diff failed: {stderr}")));
    }

    let changed_files: HashSet<String> = String::from_utf8_lossy(&output.stdout)
        .lines()
        .map(|l| l.to_string())
        .collect();

    // 2. Get deleted files specifically
    let mut cmd = Command::new("git");
    cmd.args(["diff", "--name-only", "--diff-filter=D", "HEAD"])
        .current_dir(working_dir);
    git_ops::no_window::no_window_tokio(&mut cmd);
    let del_output = cmd
        .output()
        .await
        .map_err(|e| ToolError(format!("git diff failed: {e}")))?;

    if !del_output.status.success() {
        let stderr = String::from_utf8_lossy(&del_output.stderr);
        return Err(ToolError(format!("git diff (deleted) failed: {stderr}")));
    }

    let deleted_files: HashSet<String> = String::from_utf8_lossy(&del_output.stdout)
        .lines()
        .map(|l| l.to_string())
        .collect();

    log::info!("[codebase_search] overlay: {} changed files, {} deleted files",
        changed_files.len(), deleted_files.len());

    // 3. Drop results for deleted files
    let before_len = results.len();
    results.retain(|r| !deleted_files.contains(&r.file_path));
    if results.len() < before_len {
        log::info!("[codebase_search] overlay: dropped {} deleted-file results", before_len - results.len());
    }

    // 4. For modified files in results, annotate that content may be stale.
    //    We don't replace with full-file content because SearchResultItem holds a
    //    chunk (snippet), not a whole file — replacing would bloat context.
    //    The LLM can use the `read` tool to see current content if needed.
    let mut annotated = 0usize;
    for result in results.iter_mut() {
        if changed_files.contains(&result.file_path) {
            log::info!("[codebase_search] overlay: annotating stale file {}", result.file_path);
            annotated += 1;
            result.content = format!(
                "/* NOTE: This file has local modifications in the working copy. \
                 Content below is from the index and may be stale. \
                 Use the read tool to see current content. */\n{}",
                result.content
            );
        }
    }
    if annotated > 0 {
        log::info!("[codebase_search] overlay: annotated {} results as stale", annotated);
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::context_engine::{
        ContextEngineError, GraphQueryResponse, IndexStatusResponse,
        MockContextEngine, SearchResponse, SearchResultItem,
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

    fn make_search_results() -> Vec<SearchResultItem> {
        vec![
            SearchResultItem {
                chunk_id: "c1".into(),
                content: "fn authenticate() { /* auth logic */ }".into(),
                file_path: "src/auth.rs".into(),
                language: "rust".into(),
                score: 0.95,
                source: "vector".into(),
            },
            SearchResultItem {
                chunk_id: "c2".into(),
                content: "fn validate_token() { /* token check */ }".into(),
                file_path: "src/token.rs".into(),
                language: "rust".into(),
                score: 0.82,
                source: "keyword".into(),
            },
            SearchResultItem {
                chunk_id: "c3".into(),
                content: "class AuthHandler { }".into(),
                file_path: "src/handler.ts".into(),
                language: "typescript".into(),
                score: 0.71,
                source: "hybrid".into(),
            },
        ]
    }

    #[tokio::test]
    async fn test_search_formats_results() {
        let mock = MockContextEngine::with_search_results(make_search_results());
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/repo"));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "auth"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        let out = &result.output;
        assert!(out.contains("Found 3 results"));
        assert!(out.contains("src/auth.rs"));
        assert!(out.contains("src/token.rs"));
        assert!(out.contains("src/handler.ts"));
        assert!(out.contains("score: 0.95"));
        assert!(out.contains("source: vector"));
        assert!(out.contains("fn authenticate()"));
        assert!(out.contains("```rust"));
        assert!(out.contains("```typescript"));
    }

    #[tokio::test]
    async fn test_search_cold_start() {
        let mock = MockContextEngine::not_indexed();
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/repo"));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "anything"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("currently being indexed"));
        assert!(result.output.contains("grep and glob"));
    }

    #[tokio::test]
    async fn test_search_unavailable() {
        let tool = CodebaseSearchTool::new(
            Arc::new(FailingContextEngine),
            PathBuf::from("/repo"),
        );
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "anything"}), &ctx)
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("unavailable"));
        assert!(result.output.contains("grep and glob"));
    }

    #[tokio::test]
    async fn test_search_empty_results() {
        let mock = MockContextEngine::indexed_empty();
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/repo"));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "nonexistent"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("No results found"));
        assert!(result.output.contains("nonexistent"));
    }

    #[tokio::test]
    async fn test_search_strategy_passthrough() {
        // MockContextEngine ignores args, but we verify the tool parses and doesn't error
        let mock = MockContextEngine::with_search_results(make_search_results());
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/repo"));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "auth", "strategy": "keyword"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Found 3 results"));
    }

    #[tokio::test]
    async fn test_search_limit_passthrough() {
        let mock = MockContextEngine::with_search_results(make_search_results());
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/repo"));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "auth", "limit": 5}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Found 3 results"));
    }

    #[tokio::test]
    async fn test_search_defaults() {
        let mock = MockContextEngine::with_search_results(make_search_results());
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/repo"));
        let dir = tempfile::tempdir().unwrap();
        let ctx = ToolContext::test_context(dir.path());

        // No strategy or limit — should work fine
        let result = tool
            .execute(json!({"query": "auth"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
    }

    #[tokio::test]
    async fn test_overlay_not_applied_same_dir() {
        let mock = MockContextEngine::with_search_results(make_search_results());
        let dir = tempfile::tempdir().unwrap();
        // repo_path == working_dir → no overlay
        let tool = CodebaseSearchTool::new(Arc::new(mock), dir.path().to_path_buf());
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({"query": "auth"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        // All 3 results present (no overlay filtering)
        assert!(result.output.contains("Found 3 results"));
        assert!(result.output.contains("fn authenticate()"));
    }

    #[tokio::test]
    async fn test_overlay_patches_modified_file() {
        let dir = tempfile::tempdir().unwrap();
        let dir_path = dir.path();

        // Set up a git repo with an initial commit
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(dir_path)
            .output()
            .unwrap();
        std::fs::create_dir_all(dir_path.join("src")).unwrap();
        std::fs::write(dir_path.join("src/auth.rs"), "fn old_content() {}").unwrap();
        std::process::Command::new("git")
            .args(["add", "."])
            .current_dir(dir_path)
            .output()
            .unwrap();
        std::process::Command::new("git")
            .args(["commit", "-m", "init"])
            .current_dir(dir_path)
            .env("GIT_AUTHOR_NAME", "test")
            .env("GIT_AUTHOR_EMAIL", "test@test.com")
            .env("GIT_COMMITTER_NAME", "test")
            .env("GIT_COMMITTER_EMAIL", "test@test.com")
            .output()
            .unwrap();

        // Now modify the file
        std::fs::write(dir_path.join("src/auth.rs"), "fn new_content() {}").unwrap();

        let results = vec![SearchResultItem {
            chunk_id: "c1".into(),
            content: "fn old_content() {}".into(),
            file_path: "src/auth.rs".into(),
            language: "rust".into(),
            score: 0.95,
            source: "vector".into(),
        }];
        let mock = MockContextEngine::with_search_results(results);
        // repo_path different from working_dir → overlay triggers
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/different/repo"));
        let ctx = ToolContext::test_context(dir_path);

        let result = tool
            .execute(json!({"query": "auth"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        // Content should be annotated as modified (not replaced with full file)
        assert!(result.output.contains("local modifications"));
        assert!(result.output.contains("may be stale"));
        // Original chunk content is preserved (with annotation prepended)
        assert!(result.output.contains("fn old_content()"));
    }

    #[tokio::test]
    async fn test_overlay_drops_deleted_file() {
        let dir = tempfile::tempdir().unwrap();
        let dir_path = dir.path();

        // Set up a git repo
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(dir_path)
            .output()
            .unwrap();
        std::fs::create_dir_all(dir_path.join("src")).unwrap();
        std::fs::write(dir_path.join("src/deleted.rs"), "fn gone() {}").unwrap();
        std::fs::write(dir_path.join("src/kept.rs"), "fn stay() {}").unwrap();
        std::process::Command::new("git")
            .args(["add", "."])
            .current_dir(dir_path)
            .output()
            .unwrap();
        std::process::Command::new("git")
            .args(["commit", "-m", "init"])
            .current_dir(dir_path)
            .env("GIT_AUTHOR_NAME", "test")
            .env("GIT_AUTHOR_EMAIL", "test@test.com")
            .env("GIT_COMMITTER_NAME", "test")
            .env("GIT_COMMITTER_EMAIL", "test@test.com")
            .output()
            .unwrap();

        // Delete one file
        std::fs::remove_file(dir_path.join("src/deleted.rs")).unwrap();

        let results = vec![
            SearchResultItem {
                chunk_id: "c1".into(),
                content: "fn gone() {}".into(),
                file_path: "src/deleted.rs".into(),
                language: "rust".into(),
                score: 0.90,
                source: "vector".into(),
            },
            SearchResultItem {
                chunk_id: "c2".into(),
                content: "fn stay() {}".into(),
                file_path: "src/kept.rs".into(),
                language: "rust".into(),
                score: 0.80,
                source: "keyword".into(),
            },
        ];
        let mock = MockContextEngine::with_search_results(results);
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/different/repo"));
        let ctx = ToolContext::test_context(dir_path);

        let result = tool
            .execute(json!({"query": "test"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        // Deleted file should be dropped
        assert!(!result.output.contains("src/deleted.rs"));
        assert!(!result.output.contains("fn gone()"));
        // Kept file should remain
        assert!(result.output.contains("src/kept.rs"));
        assert!(result.output.contains("Found 1 results"));
    }

    #[tokio::test]
    async fn test_overlay_passes_unmodified() {
        let dir = tempfile::tempdir().unwrap();
        let dir_path = dir.path();

        // Set up a git repo with a clean file
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(dir_path)
            .output()
            .unwrap();
        std::fs::create_dir_all(dir_path.join("src")).unwrap();
        std::fs::write(dir_path.join("src/clean.rs"), "fn original() {}").unwrap();
        std::process::Command::new("git")
            .args(["add", "."])
            .current_dir(dir_path)
            .output()
            .unwrap();
        std::process::Command::new("git")
            .args(["commit", "-m", "init"])
            .current_dir(dir_path)
            .env("GIT_AUTHOR_NAME", "test")
            .env("GIT_AUTHOR_EMAIL", "test@test.com")
            .env("GIT_COMMITTER_NAME", "test")
            .env("GIT_COMMITTER_EMAIL", "test@test.com")
            .output()
            .unwrap();

        // Don't modify any file — overlay should leave content unchanged
        let results = vec![SearchResultItem {
            chunk_id: "c1".into(),
            content: "fn original() {}".into(),
            file_path: "src/clean.rs".into(),
            language: "rust".into(),
            score: 0.90,
            source: "vector".into(),
        }];
        let mock = MockContextEngine::with_search_results(results);
        let tool = CodebaseSearchTool::new(Arc::new(mock), PathBuf::from("/different/repo"));
        let ctx = ToolContext::test_context(dir_path);

        let result = tool
            .execute(json!({"query": "test"}), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("fn original()"));
        assert!(result.output.contains("Found 1 results"));
        // No modification annotation on clean files
        assert!(!result.output.contains("local modifications"));
    }
}
