# SuperCoder

A local-first, open-source coding agent for your desktop. Bring your own LLM
key; your code stays on your machine and only ever leaves to the model provider
*you* choose — no middleman service, no lock-in.

Turn on the optional **Context Engine** and the agent navigates large
codebases structurally — tree-sitter → vector + call-graph + BM25 retrieval —
instead of guessing.

> **SuperCoder has been reimagined from the ground up.** The original (2024)
> autonomous-dev pipeline is frozen under [`v1/`](./v1) — preserved, not
> maintained or built. 

---

## Why SuperCoder

- **Local-first & fully open.** A desktop app, not a cloud product. Your source
  never transits a vendor backend — requests go straight from your machine to
  the provider whose key you configured.
- **Bring your own model.** The agent speaks the **OpenAI chat-completions** and
  **Anthropic Messages** APIs natively — no translation proxy. 
- **Graph-aware code understanding (optional).** The Context Engine indexes your
  repo into vector + call-graph + lexical search so the agent can locate code by
  structure, not just text similarity.
- **A real harness underneath.** The core is a pure-Rust agent crate with
  Ask / Plan / Coding modes, subagents, skills, tool approval, and prompt
  caching. The desktop app is one adapter over it — see
  [ARCHITECTURE.md](./ARCHITECTURE.md).

## Two ways to run

SuperCoder works the moment you add an LLM key — in-place edits,
Ask / Plan / Coding modes, checkpoints and rewind, diff review, an interactive
terminal, and a file explorer. **Zero backend required.**

Flip on the **Context Engine** (Settings → Context engine) for graph-aware,
repo-scale retrieval. It runs locally via `docker compose` and the agent's
`codebase_search` / `codebase_graph` tools query it. See
[`services/context-engine/README.md`](./services/context-engine/README.md).

## Getting started

> Prebuilt downloadable binaries are coming. For now, build from source.

**Prerequisites**

- [Rust](https://rustup.rs/) (stable) and the
  [Tauri 2 system prerequisites](https://v2.tauri.app/start/prerequisites/) for
  your OS (WebView / build tooling).
- [Node.js](https://nodejs.org/) 20+ and npm.
- (Optional, for the Context Engine) [Docker](https://docs.docker.com/get-docker/)
  with Compose.

**Run the app**

```bash
cd apps/desktop
npm install
npm run tauri:dev      # development
# or
npm run tauri:build    # produce a release bundle
```

On first launch, open **Settings** and add an LLM provider (`base_url` +
`api_key` + `model`). Then create a session, pick a folder and a mode, and go.

**(Optional) Run the Context Engine**

```bash
cd services/context-engine
cp .env.example .env    # set SUPERCODER_OPENAI_API_KEY (server-side embedding key)
docker compose up -d --build
```

Then enable **Settings → Context engine** in the app. Full instructions:
[`services/context-engine/README.md`](./services/context-engine/README.md).

## Repository layout

```
crates/
  agent/             Rust agent core — the harness (loop, tools, modes, subagents)
  git-ops/           Checkpoint / diff / restore over the working tree
apps/
  desktop/           Tauri 2 + React desktop app (thin adapter over the core)
services/
  context-engine/    Optional Go indexing service (tree-sitter → Qdrant + FalkorDB + BM25)
v1/                  Legacy 2024 codegen pipeline — frozen, not built
```

See [ARCHITECTURE.md](./ARCHITECTURE.md) for how these fit together.

## Roadmap

Present-tense — what works today — is above. Next:

- **Prebuilt releases & installers** (the CI to produce them lands next).
- **Benchmarking the harness.** A headless runner over the *same* agent core,
  with reproducible per-task execution sandboxes, to measure the harness as an
  equalizer across models and to validate the graph-retrieval localization claim.
- **Broader provider support** (the provider abstraction is built to grow).

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](./CONTRIBUTING.md) for dev
setup and repo conventions, and [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

- **Bugs & features:** GitHub Issues.
- **Questions & ideas:** GitHub Discussions.
- **Security:** please report privately — see [SECURITY.md](./SECURITY.md).

## License

[MIT](./LICENSE) © TransformerOptimus.
