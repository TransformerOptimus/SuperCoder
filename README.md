# SuperCoder

A local-first coding agent. OSS rewrite in progress.

The previous (2024) implementation is preserved under [`v1/`](./v1) and is frozen — not maintained or built.

## Layout

```
crates/      Rust agent core (agent, git-ops)
apps/        Desktop app
v1/          Legacy codegen pipeline (frozen)
```

The agent talks to LLM providers natively (OpenAI chat-completions + Anthropic
Messages API) — configure them in the app's Settings. The earlier Go LLM gateway
is preserved in git history.

Licensed under the MIT License — see [LICENSE](./LICENSE).
