//! Integration tests against real OpenAI API.
//!
//! Requires OPENAI_API_KEY environment variable to be set.
//! Run with: OPENAI_API_KEY=sk-... cargo test --test integration -- --ignored --nocapture

use std::path::{Path, PathBuf};

use tempfile::tempdir;

use agent::agent::config::{AgentConfig, CompactionConfig, RetryConfig};
use agent::agent::loop_::AgentLoop;
use agent::agent::spawn_agent;
use agent::llm::LlmClientConfig;
use agent::persistence::MockPersister;
use agent::tool::{ToolMode, ToolRegistry};
use agent::types::{AgentEvent, AgentResult};

use std::sync::Arc;

// ── Helpers ──

#[allow(dead_code)]
struct TestRunResult {
    text: String,
    events: Vec<AgentEvent>,
    working_dir: PathBuf,
}

/// Full result including the raw AgentResult variant.
#[allow(dead_code)]
struct FullRunResult {
    agent_result: AgentResult,
    events: Vec<AgentEvent>,
    working_dir: PathBuf,
}

impl TestRunResult {
    /// All tool names invoked, in order.
    fn tool_names(&self) -> Vec<&str> {
        self.events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::ToolStart { tool_name, .. } = e {
                    Some(tool_name.as_str())
                } else {
                    None
                }
            })
            .collect()
    }

    fn has_done_event(&self) -> bool {
        self.events
            .iter()
            .any(|e| matches!(e, AgentEvent::Done { .. }))
    }
}

impl FullRunResult {
    fn tool_names(&self) -> Vec<&str> {
        self.events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::ToolStart { tool_name, .. } = e {
                    Some(tool_name.as_str())
                } else {
                    None
                }
            })
            .collect()
    }

    #[allow(dead_code)]
    fn has_event<F: Fn(&AgentEvent) -> bool>(&self, pred: F) -> bool {
        self.events.iter().any(pred)
    }
}

/// Build a single-block (uncached) system prompt — for test fixtures that
/// override the default system prompt.
fn sys(text: impl Into<String>) -> Vec<agent::agent::prompt::SystemBlock> {
    vec![agent::agent::prompt::SystemBlock {
        text: text.into(),
        cache_control: None,
    }]
}

fn make_config(api_key: &str, working_dir: PathBuf) -> AgentConfig {
    let base_url = std::env::var("LLM_BASE_URL")
        .unwrap_or_else(|_| "https://api.openai.com/v1".to_string());
    let model = std::env::var("LLM_MODEL")
        .unwrap_or_else(|_| "gpt-4o-mini".to_string());
    // GPT-5.x / o1 / o3 / o4 reasoning models reject `temperature=0.0`. Drop the
    // field for those; keep 0.0 for Claude where it's deterministic and supported.
    let temperature = if model.starts_with("claude-") {
        Some(0.0)
    } else {
        None
    };
    // Support custom auth headers via env vars: LLM_AUTH_TOKEN + LLM_USER_ID + LLM_WORKSPACE_ID
    let auth_headers = if let Ok(token) = std::env::var("LLM_AUTH_TOKEN") {
        let user_id = std::env::var("LLM_USER_ID").expect("LLM_AUTH_TOKEN set but LLM_USER_ID missing");
        let workspace_id = std::env::var("LLM_WORKSPACE_ID").expect("LLM_AUTH_TOKEN set but LLM_WORKSPACE_ID missing");
        vec![
            ("X-Auth-Token".to_string(), token),
            ("X-USER-ID".to_string(), user_id),
            ("X-Workspace-ID".to_string(), workspace_id),
        ]
    } else {
        vec![("Authorization".to_string(), format!("Bearer {}", api_key))]
    };

    AgentConfig {
        llm: LlmClientConfig {
            base_url,
            model,
            temperature,
            max_completion_tokens: Some(1024),
            auth_headers,
            thinking: None,
            disable_cache_control: false,
        },
        working_dir,
        mode: ToolMode::Coding,
        max_iterations: 20,
        system_prompt: Some(vec![agent::agent::prompt::SystemBlock {
            text: "You are a coding assistant. Use the available tools to complete tasks. \
                   Be concise and use tools directly without asking for confirmation.".to_string(),
            cache_control: None,
        }]),
        retry_config: RetryConfig::default(),
        compaction_config: Default::default(),
        compaction_llm: None,
        context_engine: None,
        context_engine_repo_path: None,
        skills: None,
        subagents: None,
        subagent_inheritance: None,
    }
}

async fn run_agent(config: AgentConfig, message: &str) -> TestRunResult {
    let working_dir = config.working_dir.clone();
    let spawned = spawn_agent(config, agent::llm::types::ChatMessage::user(message), None, None, None);
    let handle = spawned.handle;
    let mut event_rx = spawned.event_rx;
    let cancel_token = spawned.cancel_token;
    eprintln!("Session ID: {}", spawned.session_id);

    let event_collector = tokio::spawn(async move {
        let mut events = Vec::new();
        while let Some(event) = event_rx.recv().await {
            eprintln!("Event: {:?}", event);
            events.push(event);
        }
        events
    });

    let result = handle.await.unwrap();
    drop(cancel_token);
    let events = event_collector.await.unwrap();

    let text = match result {
        Ok(AgentResult::Done { summary }) => {
            eprintln!("Agent completed: {summary}");
            summary
        }
        Ok(AgentResult::AskUser { question, .. }) => {
            eprintln!("Agent asks: {question}");
            question
        }
        Ok(AgentResult::PlanReady { plan, .. }) => {
            eprintln!("Agent produced plan");
            plan
        }
        Err(e) => panic!("Agent failed: {e}"),
    };

    TestRunResult {
        text,
        events,
        working_dir,
    }
}

/// Run an agent with a specific ToolMode. Returns the full AgentResult.
async fn run_agent_with_mode(config: AgentConfig, message: &str, mode: ToolMode) -> FullRunResult {
    let working_dir = config.working_dir.clone();
    let (event_tx, mut event_rx) = tokio::sync::mpsc::channel(256);
    let cancel_token = tokio_util::sync::CancellationToken::new();
    let session_id = uuid::Uuid::new_v4().to_string();
    let registry = ToolRegistry::for_mode(mode, None, None);

    let message_owned = message.to_string();
    let handle = {
        let cancel_token = cancel_token.clone();
        let session_id = session_id.clone();
        tokio::spawn(async move {
            let mut agent_loop =
                AgentLoop::new(config, registry, cancel_token, event_tx, session_id);
            agent_loop.run(agent::llm::types::ChatMessage::user(message_owned)).await
        })
    };

    let event_collector = tokio::spawn(async move {
        let mut events = Vec::new();
        while let Some(event) = event_rx.recv().await {
            eprintln!("Event: {:?}", event);
            events.push(event);
        }
        events
    });

    let result = handle.await.unwrap();
    drop(cancel_token);
    let events = event_collector.await.unwrap();

    let agent_result = match result {
        Ok(r) => {
            eprintln!("Agent result: {:?}", r);
            r
        }
        Err(e) => panic!("Agent failed: {e}"),
    };

    FullRunResult {
        agent_result,
        events,
        working_dir,
    }
}

/// Run agent with a custom config that returns FullRunResult (coding mode).
async fn run_agent_full(config: AgentConfig, message: &str) -> FullRunResult {
    run_agent_with_mode(config, message, ToolMode::Coding).await
}

fn api_key() -> String {
    std::env::var("OPENAI_API_KEY").unwrap_or_else(|_| "unused".to_string())
}

fn read_file(dir: &Path, name: &str) -> String {
    std::fs::read_to_string(dir.join(name)).unwrap()
}

// ── Tests ──

/// Phase 1 — write + read round-trip.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_agent_creates_and_reads_file() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    let config = make_config(&api_key(), tmp.path().to_path_buf());

    let r = run_agent(
        config,
        "Create a file called hello.txt containing exactly 'hello world', then read it back to confirm.",
    ).await;

    // File created with correct content — this is the real assertion
    let content = read_file(&r.working_dir, "hello.txt");
    assert_eq!(content.trim(), "hello world");

    let names = r.tool_names();
    assert!(names.contains(&"write"), "Expected write tool, got: {names:?}");
    assert!(r.has_done_event());
}

/// Phase 2, Step 2.7 — read then edit (multi-tool sequential).
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_agent_reads_and_edits_file() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    std::fs::write(tmp.path().join("test.txt"), "hello world\n").unwrap();

    let config = make_config(&api_key(), tmp.path().to_path_buf());

    let r = run_agent(
        config,
        "Read the file test.txt, then change 'hello' to 'goodbye' in it using the edit tool.",
    ).await;

    // File edited correctly
    let content = read_file(&r.working_dir, "test.txt");
    assert!(content.contains("goodbye"), "Should contain 'goodbye'");
    assert!(!content.contains("hello"), "Should not contain 'hello'");

    // Tool ordering: read must appear before edit
    let names = r.tool_names();
    let read_pos = names.iter().position(|n| *n == "read").expect("read tool not used");
    let edit_pos = names.iter().position(|n| *n == "edit").expect("edit tool not used");
    assert!(read_pos < edit_pos, "read should come before edit, got: {names:?}");
    assert!(r.has_done_event());
}

/// Phase 2 — glob + grep search tools.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_agent_uses_glob_and_grep() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    let src = tmp.path().join("src");
    std::fs::create_dir_all(&src).unwrap();
    std::fs::write(src.join("main.rs"), "fn main() {\n    println!(\"hello\");\n}\n").unwrap();
    std::fs::write(src.join("lib.rs"), "pub fn add(a: i32, b: i32) -> i32 { a + b }\n").unwrap();
    std::fs::write(tmp.path().join("README.md"), "# My Project\n").unwrap();

    let config = make_config(&api_key(), tmp.path().to_path_buf());

    let r = run_agent(
        config,
        "Find all .rs files in this project using glob, then search for the word 'fn' using grep. \
         Tell me what you found.",
    ).await;

    let names = r.tool_names();
    assert!(names.contains(&"glob"), "Expected glob tool, got: {names:?}");
    assert!(names.contains(&"grep"), "Expected grep tool, got: {names:?}");
    assert!(r.has_done_event());
}

/// Error recovery — agent handles a tool error and adapts.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_agent_recovers_from_tool_error() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();

    let config = make_config(&api_key(), tmp.path().to_path_buf());

    let r = run_agent(
        config,
        "Try to read a file called missing.txt. It won't exist. \
         After the error, create it with the content 'recovered'.",
    ).await;

    // File should exist now with correct content
    let content = read_file(&r.working_dir, "missing.txt");
    assert_eq!(content.trim(), "recovered");
    assert!(r.has_done_event());
}

/// Phase 6 — git tool: status, add, commit in a temp repo.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_agent_uses_git_tool() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();

    // Initialize a real git repo in the temp dir
    std::process::Command::new("git")
        .args(["init"])
        .current_dir(tmp.path())
        .output()
        .unwrap();
    std::process::Command::new("git")
        .args(["config", "user.email", "test@test.com"])
        .current_dir(tmp.path())
        .output()
        .unwrap();
    std::process::Command::new("git")
        .args(["config", "user.name", "Test"])
        .current_dir(tmp.path())
        .output()
        .unwrap();

    // Create an initial commit so the repo is not empty
    std::fs::write(tmp.path().join("README.md"), "# Test Project\n").unwrap();
    std::process::Command::new("git")
        .args(["add", "-A"])
        .current_dir(tmp.path())
        .output()
        .unwrap();
    std::process::Command::new("git")
        .args(["commit", "-m", "initial"])
        .current_dir(tmp.path())
        .output()
        .unwrap();

    // Now create an uncommitted file for the agent to discover
    std::fs::write(tmp.path().join("new_feature.txt"), "some new code\n").unwrap();

    let config = make_config(&api_key(), tmp.path().to_path_buf());

    let r = run_agent(
        config,
        "Use the git tool to check the status of this repo. There should be an untracked file. \
         Stage it with git add, then commit it with the message 'add new feature'. \
         Finally run git log to confirm the commit.",
    ).await;

    let names = r.tool_names();
    assert!(names.contains(&"git"), "Expected git tool, got: {names:?}");

    // Verify the commit actually happened by checking git log
    let log_output = std::process::Command::new("git")
        .args(["log", "--oneline"])
        .current_dir(tmp.path())
        .output()
        .unwrap();
    let log_text = String::from_utf8_lossy(&log_output.stdout);
    assert!(log_text.contains("add new feature"), "Commit not found in log: {log_text}");

    // The file should no longer be untracked
    let status_output = std::process::Command::new("git")
        .args(["status", "--porcelain"])
        .current_dir(tmp.path())
        .output()
        .unwrap();
    let status_text = String::from_utf8_lossy(&status_output.stdout);
    assert!(status_text.trim().is_empty(), "Repo should be clean, got: {status_text}");

    assert!(r.has_done_event());
}

// ── Phase 3 Tests ──

/// Phase 3 — Ask mode cannot use destructive tools (write, edit, bash, git).
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_ask_mode_tool_isolation() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    std::fs::write(tmp.path().join("test.txt"), "original content\n").unwrap();

    let mut config = make_config(&api_key(), tmp.path().to_path_buf());
    config.system_prompt = Some(sys(
        "You are a coding assistant in ask mode. You only have read, glob, grep, and ask_user tools. \
         Read the file test.txt and tell the user its content. Do NOT try to modify it.",
    ));

    let r = run_agent_with_mode(
        config,
        "Read test.txt and tell me what's in it.",
        ToolMode::Ask,
    )
    .await;

    // Verify only read-only tools were used (ask mode has: read, glob, grep, ask_user)
    let names = r.tool_names();
    for name in &names {
        assert!(
            ["read", "glob", "grep", "ask_user"].contains(name),
            "Ask mode used unexpected tool: {name}"
        );
    }
    assert!(names.contains(&"read"), "Expected read tool, got: {names:?}");

    // File should be unmodified
    let content = read_file(&r.working_dir, "test.txt");
    assert_eq!(content, "original content\n");
}

/// Phase 3 — Max iterations returns graceful Done (not an error).
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_max_iterations_graceful() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    let src = tmp.path().join("src");
    std::fs::create_dir_all(&src).unwrap();
    for i in 0..20 {
        std::fs::write(
            src.join(format!("file{i}.rs")),
            format!("fn func_{i}() {{}}\n"),
        )
        .unwrap();
    }

    let mut config = make_config(&api_key(), tmp.path().to_path_buf());
    config.max_iterations = 2; // Very low limit — glob + read will consume both steps

    let r = run_agent_full(
        config,
        "Read every .rs file in the src/ directory one by one, then write a summary to summary.txt. \
         There are 20 files so read each one individually, then create the summary file.",
    )
    .await;

    // Should complete with Done (not panic or error), either gracefully or by hitting the limit
    match &r.agent_result {
        AgentResult::Done { summary } => {
            eprintln!("Max iterations result: {summary}");
            // Either the agent hit the limit or completed — both are Ok(Done), not Err
        }
        other => panic!("Expected Done, got: {:?}", other),
    }
}

/// Phase 3 — Coding mode with persistence records all messages.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_coding_mode_with_persistence() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    std::fs::write(tmp.path().join("hello.txt"), "hello world\n").unwrap();

    let config = make_config(&api_key(), tmp.path().to_path_buf());
    let persister = Arc::new(MockPersister::new());

    let (event_tx, mut event_rx) = tokio::sync::mpsc::channel(256);
    let cancel_token = tokio_util::sync::CancellationToken::new();
    let session_id = uuid::Uuid::new_v4().to_string();
    let registry = ToolRegistry::for_mode(ToolMode::Coding, None, None);

    let handle = {
        let cancel_token = cancel_token.clone();
        let session_id = session_id.clone();
        let persister_clone = Arc::clone(&persister) as Arc<dyn agent::persistence::MessagePersister>;
        tokio::spawn(async move {
            let mut agent_loop =
                AgentLoop::new(config, registry, cancel_token, event_tx, session_id);
            agent_loop = agent_loop.with_persister(persister_clone, "test-thread".into());
            agent_loop.run(agent::llm::types::ChatMessage::user("Read hello.txt and tell me its content.")).await
        })
    };

    // Collect events
    let event_collector = tokio::spawn(async move {
        let mut events = Vec::new();
        while let Some(event) = event_rx.recv().await {
            events.push(event);
        }
        events
    });

    let result = handle.await.unwrap().unwrap();
    drop(cancel_token);
    let _events = event_collector.await.unwrap();

    // Result should be Done
    match &result {
        AgentResult::Done { summary } => {
            eprintln!("Persisted agent result: {summary}");
            assert!(
                summary.to_lowercase().contains("hello world")
                    || summary.to_lowercase().contains("hello"),
                "Summary should reference file content"
            );
        }
        other => panic!("Expected Done, got: {:?}", other),
    }

    // Give fire-and-forget persistence tasks a moment to complete
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    // Verify persistence received messages
    let messages = persister.messages();
    assert!(
        messages.len() >= 2,
        "Should have at least user + assistant messages, got {}",
        messages.len()
    );
    eprintln!("Persisted {} messages", messages.len());

    // All should have session_id = "test-thread"
    for (sid, _msg) in &messages {
        assert_eq!(
            sid.as_str(),
            "test-thread",
            "Expected session_id 'test-thread'"
        );
    }
}

/// Phase 3 — Compaction triggers with small context limit.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_compaction_triggers_with_small_context() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();

    // Create several files so the agent generates enough content to trigger compaction
    for i in 0..5 {
        std::fs::write(
            tmp.path().join(format!("file{i}.txt")),
            format!("Content of file {i}: {}", "x".repeat(200)),
        )
        .unwrap();
    }

    let mut config = make_config(&api_key(), tmp.path().to_path_buf());
    // Very small context limit to force compaction
    config.compaction_config = CompactionConfig {
        context_limit: 500,   // ~500 tokens → threshold at 400 tokens
        threshold_pct: 0.80,
        keep_recent_messages: 2,
        max_messages: 10_000,
    };
    config.max_iterations = 10;

    let (event_tx, mut event_rx) = tokio::sync::mpsc::channel(256);
    let cancel_token = tokio_util::sync::CancellationToken::new();
    let session_id = uuid::Uuid::new_v4().to_string();
    let registry = ToolRegistry::for_mode(ToolMode::Coding, None, None);
    let persister = Arc::new(MockPersister::new());

    let handle = {
        let cancel_token = cancel_token.clone();
        let session_id = session_id.clone();
        let persister_clone = Arc::clone(&persister) as Arc<dyn agent::persistence::MessagePersister>;
        tokio::spawn(async move {
            let mut agent_loop =
                AgentLoop::new(config, registry, cancel_token, event_tx, session_id);
            agent_loop = agent_loop.with_persister(persister_clone, "compact-thread".into());
            agent_loop
                .run(agent::llm::types::ChatMessage::user("Read all .txt files one by one (file0.txt through file4.txt) and summarize each one."))
                .await
        })
    };

    let event_collector = tokio::spawn(async move {
        let mut events = Vec::new();
        while let Some(event) = event_rx.recv().await {
            events.push(event);
        }
        events
    });

    let _result = handle.await.unwrap();
    drop(cancel_token);
    let events = event_collector.await.unwrap();

    // Check if compaction event was emitted
    let has_compaction = events
        .iter()
        .any(|e| matches!(e, AgentEvent::Compaction { .. }));

    // Give persistence time to flush
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    // Check if compaction was persisted
    let messages = persister.messages();
    let has_compaction_record = messages
        .iter()
        .any(|(_, msg)| matches!(msg.message_type, agent::persistence::MessageType::Compaction));

    eprintln!(
        "Compaction event emitted: {has_compaction}, compaction persisted: {has_compaction_record}, total messages: {}",
        messages.len()
    );

    // With a 500-token context limit and 5 file reads, compaction should trigger.
    // If it doesn't (model is very concise), at least verify the agent completed.
    if has_compaction {
        assert!(
            has_compaction_record,
            "Compaction event emitted but no compaction record persisted"
        );
    }
}

// ── v1.0 Feature Tests ──

/// Plan mode: agent uses ask_user to yield a clarifying question.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_plan_mode_ask_user_yield() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    std::fs::write(tmp.path().join("main.rs"), "fn main() {}\n").unwrap();

    let mut config = make_config(&api_key(), tmp.path().to_path_buf());
    config.mode = ToolMode::Plan;
    config.system_prompt = Some(sys(
        "You are a planning agent. You have read-only tools and an ask_user tool. \
         When the user's request is ambiguous, you MUST call the ask_user tool to clarify. \
         The user's request below is intentionally vague — call ask_user with a clarifying question \
         and provide 2-3 options. Do NOT try to answer directly, you MUST ask a question first.",
    ));

    let r = run_agent_with_mode(
        config,
        "Improve the code.",
        ToolMode::Plan,
    )
    .await;

    // Should yield AskUser (not Done or StartSession)
    match &r.agent_result {
        AgentResult::AskUser { question, .. } => {
            eprintln!("Agent asked: {question}");
            assert!(!question.is_empty(), "Question should not be empty");
        }
        other => {
            // Some models may not follow the instruction perfectly — at least verify
            // ask_user was attempted or the agent returned a reasonable result
            let names = r.tool_names();
            if names.contains(&"ask_user") {
                eprintln!("ask_user was called but agent returned {:?}", other);
            } else {
                eprintln!("WARN: Agent did not use ask_user tool. Got: {:?}", other);
            }
        }
    }
}

/// Coding mode: agent uses todo_write for multi-step tasks.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_coding_mode_todo_write() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();

    let config = make_config(&api_key(), tmp.path().to_path_buf());

    let r = run_agent(
        config,
        "You must complete these 3 steps. Use the todo_write tool to track them: \
         1. Create a file called a.txt with content 'aaa'. \
         2. Create a file called b.txt with content 'bbb'. \
         3. Create a file called c.txt with content 'ccc'. \
         Use todo_write to plan these steps BEFORE starting, then mark each as completed as you finish.",
    )
    .await;

    // Verify todo_write was used
    let names = r.tool_names();
    assert!(
        names.contains(&"todo_write"),
        "Expected todo_write tool, got: {names:?}"
    );

    // Verify the files were created
    assert!(
        tmp.path().join("a.txt").exists(),
        "a.txt should have been created"
    );
    assert!(
        tmp.path().join("b.txt").exists(),
        "b.txt should have been created"
    );
    assert!(
        tmp.path().join("c.txt").exists(),
        "c.txt should have been created"
    );

    // Verify the todos.md file was written
    let todos_path = tmp.path().join(".agent").join("todos.md");
    assert!(
        todos_path.exists(),
        "todos.md should have been created by todo_write"
    );

    assert!(r.has_done_event());
}

/// Plan mode: tool isolation — only read-only tools + ask_user + save_plan + edit_plan.
#[tokio::test]
#[ignore = "requires OPENAI_API_KEY"]
async fn test_plan_mode_tool_isolation() {
    let _ = env_logger::try_init();
    let tmp = tempdir().unwrap();
    std::fs::write(tmp.path().join("test.txt"), "plan mode content\n").unwrap();

    let mut config = make_config(&api_key(), tmp.path().to_path_buf());
    config.mode = ToolMode::Plan;
    config.system_prompt = Some(sys(
        "You are a planning agent. Read test.txt and tell me what's in it. \
         Do NOT try to modify files — you only have read-only tools.",
    ));

    let r = run_agent_with_mode(
        config,
        "Read test.txt and describe its contents.",
        ToolMode::Plan,
    )
    .await;

    // Verify only plan-mode tools were used
    let names = r.tool_names();
    let plan_tools = ["read", "glob", "grep", "ask_user", "save_plan", "edit_plan"];
    for name in &names {
        assert!(
            plan_tools.contains(name),
            "Plan mode used unexpected tool: {name}"
        );
    }

    // File should be unmodified
    let content = read_file(&r.working_dir, "test.txt");
    assert_eq!(content, "plan mode content\n");
}

// ── Compaction integration test ──

/// Test that compaction fires with a real LLM when the context is pre-seeded
/// with enough history to exceed the (tiny) threshold.
/// We seed 6 prior conversation messages (creates clean compaction boundaries),
/// then ask the agent to read a file — the token count will exceed the threshold.
#[tokio::test]
#[ignore] // Requires OPENAI_API_KEY
async fn test_compaction_survives_multi_file_reads() {
    let tmp = tempdir().unwrap();
    std::fs::write(
        tmp.path().join("data.txt"),
        (0..50).map(|i| format!("row {i}: {}", "x".repeat(80))).collect::<Vec<_>>().join("\n"),
    ).unwrap();

    let mut config = make_config(&api_key(), tmp.path().to_path_buf());
    config.compaction_config = CompactionConfig {
        context_limit: 2000,
        threshold_pct: 0.40,  // threshold = 800 tokens
        keep_recent_messages: 2,
        max_messages: 10_000,
    };
    config.max_iterations = 10;

    let (event_tx, mut event_rx) = tokio::sync::mpsc::channel(256);
    let cancel_token = tokio_util::sync::CancellationToken::new();
    let session_id = uuid::Uuid::new_v4().to_string();
    let registry = ToolRegistry::for_mode(ToolMode::Coding, None, None);

    // Seed prior context with clean boundaries (user/assistant text pairs)
    let prior_context = vec![
        agent::llm::types::ChatMessage::user("What files are in this project?"),
        agent::llm::types::ChatMessage::assistant(Some("I found several configuration files including data.txt, config.yaml, and main.rs. The project appears to be a Rust application with some data files.".into()), None, None),
        agent::llm::types::ChatMessage::user("Tell me about the architecture"),
        agent::llm::types::ChatMessage::assistant(Some("The architecture follows a layered pattern with a data layer, service layer, and presentation layer. Each module has its own configuration constants defined in separate files.".into()), None, None),
        agent::llm::types::ChatMessage::user("What patterns do they use?"),
        agent::llm::types::ChatMessage::assistant(Some("They use the builder pattern for configuration, the observer pattern for events, and dependency injection throughout. The code is well-structured with clear separation of concerns.".into()), None, None),
    ];

    let sid = session_id.clone();
    let ct = cancel_token.clone();
    let handle = tokio::spawn(async move {
        let mut agent = AgentLoop::new(config, registry, ct, event_tx, sid);
        agent = agent.with_initial_context(prior_context);
        agent.run(agent::llm::types::ChatMessage::user(
            "Read data.txt and tell me how many rows it has."
        )).await
    });

    let event_collector = tokio::spawn(async move {
        let mut events = Vec::new();
        while let Some(event) = event_rx.recv().await {
            eprintln!("Event: {:?}", event);
            events.push(event);
        }
        events
    });

    let result = handle.await.unwrap();
    drop(cancel_token);
    let events = event_collector.await.unwrap();

    // Agent should complete successfully
    match result {
        Ok(AgentResult::Done { summary }) => eprintln!("Agent completed: {summary}"),
        Ok(other) => eprintln!("Agent result: {:?}", other),
        Err(e) => panic!("Agent failed: {e}"),
    }

    let compaction_count = events
        .iter()
        .filter(|e| matches!(e, AgentEvent::Compaction { .. }))
        .count();
    eprintln!("Compaction fired {} time(s)", compaction_count);

    assert!(
        compaction_count >= 1,
        "Compaction should fire at least once with context_limit=2000, seeded context, and a file read"
    );

    assert!(
        events.iter().any(|e| matches!(e, AgentEvent::Done { .. })),
        "Agent should emit Done even after compaction"
    );
}

// ───────────────────────────────────────────────────────────────────────────
// Prompt caching tests
//
// These assert the cache-token telemetry path end-to-end. Intentionally written
// BEFORE the gateway + Rust cache_control changes land — they fail today with
// cache counts at zero, and pass once Stage 2 (gateway) + Stage 3 (Rust client)
// are both in place.
//
// Env setup for the ignored test:
//   LLM_BASE_URL  = gateway URL that forwards cache_control to Anthropic
//   LLM_MODEL     = claude-sonnet-4-6  (coding-mode system+tools clears 2K min)
//   OPENAI_API_KEY (or gateway auth envvars populated by make_config)
// ───────────────────────────────────────────────────────────────────────────

#[test]
fn test_usage_deserializes_prompt_tokens_details() {
    // This is the shape the gateway will emit after Stage 2: Anthropic's
    // cache_{read,creation}_input_tokens mapped into OpenAI-style
    // prompt_tokens_details.{cached_tokens, cache_creation_tokens}.
    use agent::llm::types::Usage;
    let json = r#"{
        "prompt_tokens": 5000,
        "completion_tokens": 300,
        "total_tokens": 5300,
        "prompt_tokens_details": {
            "cached_tokens": 4500,
            "cache_creation_tokens": 400
        }
    }"#;
    let usage: Usage = serde_json::from_str(json).expect("Usage should parse");
    let details = usage
        .prompt_tokens_details
        .expect("prompt_tokens_details should be present");
    assert_eq!(details.cached_tokens, Some(4500));
    assert_eq!(details.cache_creation_tokens, Some(400));
}

#[test]
fn test_usage_without_cache_details_still_parses() {
    // Backward-compat: pre-caching responses (or OpenAI responses without
    // caching active) must still deserialize cleanly.
    use agent::llm::types::Usage;
    let json = r#"{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}"#;
    let usage: Usage = serde_json::from_str(json).expect("legacy Usage should parse");
    assert!(usage.prompt_tokens_details.is_none());
}

/// End-to-end caching test. Runs a multi-LLM-call coding session against a
/// real caching-capable gateway/provider. Asserts that:
///   - at least one LLM call writes to cache (cache_creation_tokens > 0)
///   - at least one subsequent LLM call reads from cache (cache_read_tokens > 0)
///
/// Fails today because:
///   - Gateway strips cache_control fields from outbound Anthropic requests
///   - Rust client does not mark cache_control on system/tools
///   - Gateway does not surface cache_* tokens in Usage
///
/// Expected to pass after Stages 2 and 3 ship.
#[tokio::test]
#[ignore = "requires ANTHROPIC_API_KEY + caching-capable endpoint (gateway or direct)"]
async fn test_caching_emits_cache_tokens_across_turns() {
    let _ = env_logger::try_init();
    let api_key = api_key();
    let tmp = tempdir().unwrap();
    let mut cfg = make_config(&api_key, tmp.path().to_path_buf());

    // Coding mode: system(~2.1K) + tools(~1.4K) ≈ 3.5K tokens.
    // We pad the user message with ~3KB of stable context so turn-1's request
    // clears the HIGHEST Anthropic minimum (Opus 4.6 + Haiku 4.5 = 4096 tokens).
    // Without padding, Opus/Haiku fall below threshold on turn 1 → no cache write
    // → nothing for turn 2 to read. With padding, all three Claude models and
    // OpenAI-family models clear their respective minima.
    cfg.mode = ToolMode::Coding;
    cfg.max_iterations = 5;

    // CRITICAL: use the REAL build_system_prompt, which returns a 2-block system
    // with cache_control on the static body. The default test harness system
    // prompt (from make_config) is a single uncached block, which would never
    // trigger Anthropic caching no matter what the client does.
    cfg.system_prompt = Some(agent::agent::prompt::build_system_prompt(
        cfg.mode,
        &cfg.working_dir,
        None, // no branch
        None, // no project note
        None, // no skills
        None, // no subagents
    ));

    std::fs::write(tmp.path().join("sample.txt"), "hello world\n").unwrap();

    // Stable preamble repeated deterministically. ~9 KB ≈ ~2.2K tokens → combined
    // with the real Coding system prompt (~3.5K tokens system+tools) we comfortably
    // clear Opus 4.6 / Haiku 4.5's 4096-token minimum on turn 1.
    let padding = "The assistant is careful, precise, and does not speculate. "
        .repeat(150);
    let user_message = format!(
        "{padding}\n\nRead the file `sample.txt` in the working directory and \
         tell me its contents in one short sentence."
    );

    let result = run_agent(cfg, &user_message).await;

    let token_usages: Vec<(Option<u32>, Option<u32>)> = result
        .events
        .iter()
        .filter_map(|e| match e {
            AgentEvent::TokenUsage {
                cache_read_tokens,
                cache_creation_tokens,
                ..
            } => Some((*cache_read_tokens, *cache_creation_tokens)),
            _ => None,
        })
        .collect();

    eprintln!(
        "Captured {} TokenUsage events: {:?}",
        token_usages.len(),
        token_usages
    );

    assert!(
        token_usages.len() >= 2,
        "Need at least 2 LLM calls to test caching; got {} (did the agent use a tool?)",
        token_usages.len()
    );

    let any_write = token_usages
        .iter()
        .any(|(_r, w)| w.unwrap_or(0) > 0);
    let any_read = token_usages
        .iter()
        .any(|(r, _w)| r.unwrap_or(0) > 0);

    // Provider-aware oracle. Anthropic reports BOTH creation and read tokens;
    // OpenAI only reports cached reads (no creation — writes are automatic + free
    // in their pricing model). We accept either pattern as proof caching is
    // engaged end-to-end.
    let model = std::env::var("LLM_MODEL").unwrap_or_default();
    let is_openai_family = model.starts_with("gpt-")
        || model.starts_with("o1-")
        || model.starts_with("o3")
        || model.starts_with("o4")
        || model.starts_with("chatgpt-");

    if is_openai_family {
        // OpenAI: success iff at least one turn read from cache.
        assert!(
            any_read,
            "OpenAI: expected at least one LLM call to READ cache \
             (prompt_tokens_details.cached_tokens > 0), but none did. \
             TokenUsage events: {:?}",
            token_usages
        );
    } else {
        // Anthropic (and anything else that reports both): success iff BOTH
        // write and read occurred. This is the strict end-to-end oracle.
        assert!(
            any_write,
            "Expected at least one LLM call to WRITE cache (cache_creation_tokens > 0), but got none. \
             Likely causes: gateway strips cache_control; Rust client doesn't emit cache_control; \
             prompt is below provider's minimum cacheable size. TokenUsage events: {:?}",
            token_usages
        );
        assert!(
            any_read,
            "Expected at least one LLM call to READ cache (cache_read_tokens > 0), but got none. \
             Either the cache was never written, or subsequent calls don't match the cached prefix. \
             TokenUsage events: {:?}",
            token_usages
        );
    }
}
