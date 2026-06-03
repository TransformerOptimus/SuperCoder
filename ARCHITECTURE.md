# Architecture

SuperCoder is a polyglot monorepo built around one idea: **the agent core is the
spine, everything else is an adapter over it.** The Rust agent crate owns the
loop, the tools, and the provider protocols. The desktop app is one adapter; a
headless benchmark runner (planned) will be another. No adapter reaches into the
core's internals.

```
                      ┌─────────────────────────────────────────┐
                      │            apps/desktop                   │
                      │   React UI  ◄──Tauri IPC──►  agent_bridge │
                      │  (sessions, diff,           (SQLite-only  │
                      │   terminal, plan,            persistence, │
                      │   settings)                  checkpoints) │
                      └───────────────────┬───────────────────────┘
                                          │ drives
                                          ▼
                  ┌───────────────────────────────────────────────┐
                  │              crates/agent  (the harness)        │
                  │  loop · tools · Ask/Plan/Coding modes ·         │
                  │  subagents · skills · approval · prompt cache   │
                  │                                                 │
                  │   ┌──────────────┐   ┌──────────────────────┐  │
                  │   │ llm/ (native)│   │ checkpoints via      │  │
                  │   │ OpenAI       │   │ crates/git-ops       │  │
                  │   │ Anthropic    │   │ (snapshot/diff/      │  │
                  │   └──────┬───────┘   │  restore working tree)│  │
                  └──────────┼───────────┴──────────┬────────────┘
                             │                       │ (optional)
                             ▼                       ▼
                 ┌────────────────────┐   ┌────────────────────────────┐
                 │  LLM provider       │   │  services/context-engine    │
                 │  (your endpoint,    │   │  tree-sitter → Qdrant +     │
                 │   your key)         │   │  FalkorDB + BM25, :8106      │
                 └────────────────────┘   └────────────────────────────┘
```

## Components

### `crates/agent` — the agent core (the spine)

A pure-Rust crate with no dependency on the desktop app, no chat/UI coupling, and
no network framework assumptions. It contains:

- **The agent loop** — turn orchestration, streaming events, compaction, token
  accounting.
- **Tools** — file read/write/edit, bash, search, plus the optional
  `codebase_search` / `codebase_graph` tools (gated on the Context Engine).
- **Modes** — Ask, Plan, and Coding. The mode is fixed per session and chosen at
  session creation. `save_plan` / `ask_user` yield to the adapter, not to any
  remote service.
- **Subagents & skills** — spawnable child agents (with their own approval
  routing) and reusable skill definitions under `default-subagents/`.
- **Providers** — a `Provider` enum (OpenAI | Anthropic). `llm/openai.rs` and
  `llm/anthropic.rs` speak each wire format natively: URL, auth, SSE parsing,
  prompt-cache markers, and extended thinking are owned here. There is **no
  translation proxy** in the runtime path.
- **Persistence trait** — the core defines a message-persistence interface keyed
  by `session_id`; the adapter supplies the implementation.

The core never decides *where* messages are stored or *how* tool approvals are
surfaced — it calls trait methods the adapter implements.

### `crates/git-ops` — checkpoints over the working tree

Since the agent edits the project **in place** (no per-session git worktrees),
`git-ops` provides file-snapshot checkpoints: `backup_file` captures a file's
prior contents (first-write-wins per turn), `restore_to` reverts exactly the
files a turn touched, and `diff_turn` / `list` / `delete_from` drive diff review
and rewind. Restore is bounded to the project root (`is_within_root`) so a
checkpoint can never write outside the session folder.

### `apps/desktop` — the desktop adapter

- **`src-tauri/src/agent_bridge/`** — the bridge between Tauri and the agent core.
  `commands.rs` exposes the Tauri commands the UI calls; `events.rs` relays agent
  events to the frontend; `db.rs` is the **local SQLite** store
  (`agent_data.db`) — the sole datastore. There is no remote persistence. Each
  session gets a `checkpoint_dir` under the app data directory, outside the user's
  project.
- **`src/`** — a React (Vite + antd + zustand) UI: session-list sidebar, diff
  review, interactive terminal, file explorer, plan panel, subagent/skill/
  permission dialogs, and Settings (LLM providers + Context Engine toggle).

The bridge implements the core's persistence and approval traits; it does not
reach inside the loop.

### `services/context-engine` — optional graph-aware retrieval

A Go service (built and run via `docker compose`) that indexes a repository with
tree-sitter and serves semantic + structural search:

- **Stores:** Qdrant (vectors), FalkorDB (call/symbol graph), BM25 (lexical),
  Postgres (metadata), Redis (queue). A worker (Asynq) does the indexing.
- **Streaming sync:** the app streams the repo up on session start and
  incrementally thereafter — `/index/diff` → `/index/stream` → `/index/sync-complete`
  (gzipped NDJSON). A Merkle tree on a local-disk volume makes incremental syncs
  cheap.
- **Opt-in:** disabled by default. When off, `codebase_search` / `codebase_graph`
  are gated off and the agent runs fully zero-backend. Enabled via a Settings
  toggle + editable `base_url` (default `http://127.0.0.1:8106`).
- **Config:** environment prefix `SUPERCODER_`. The server-side embedding key
  lives in the service's `.env`, never in the app.

### `v1/` — frozen legacy

The 2024 autonomous-dev codegen pipeline, preserved verbatim for history. It is
not built, tested, or maintained, and shares nothing with the current code beyond
the name.

## Data & trust boundaries

- **Your code → LLM provider.** Source and prompts go directly from your machine
  to the endpoint you configured in Settings. No SuperCoder-operated server sits
  in between.
- **Your code → Context Engine (optional, local).** When enabled, the repo is
  streamed to the locally-running Docker stack. The embedding key it uses is the
  service's own, kept server-side.
- **Local persistence.** Sessions, messages, and checkpoints live in local SQLite
  and the app data directory — nothing is synced anywhere.

## Why this shape

Keeping the harness as a standalone crate with trait-shaped seams means the same
core can be driven by the desktop app today and by a headless benchmark runner
later (see the Roadmap in the [README](./README.md)) without forking logic. The
provider protocols living *in* the core — rather than behind a gateway — is what
makes "bring your own model, no middleman" true at the architecture level, not
just the marketing level.
