# Contributing to SuperCoder

Thanks for your interest in contributing. This guide covers how to set up a dev
environment, where things live, and what we expect in a pull request.

By participating you agree to abide by our [Code of Conduct](./CODE_OF_CONDUCT.md).

## Where to start

- **Bugs & feature requests:** open a GitHub Issue.
- **Questions & ideas:** start a GitHub Discussion.
- **Security issues:** do **not** open a public issue — see [SECURITY.md](./SECURITY.md).

If you're planning a non-trivial change, open an issue or discussion first so we
can agree on the approach before you write code.

## Repository layout

```
crates/agent/          Rust agent core (the harness) — loop, tools, modes, providers
crates/git-ops/        Checkpoint / diff / restore over the working tree
apps/desktop/          Tauri 2 + React desktop app (src-tauri = Rust bridge, src = React)
services/context-engine/  Optional Go indexing service (run via docker compose)
v1/                    Frozen 2024 legacy pipeline — do not modify
```

See [ARCHITECTURE.md](./ARCHITECTURE.md) for how the pieces relate. The agent
core is intentionally decoupled from the app: keep adapter concerns
(persistence, UI, IPC) in `apps/desktop`, and keep loop/tool/provider logic in
`crates/agent`.

## Prerequisites

- [Rust](https://rustup.rs/) (stable; the crates use edition 2021) and the
  [Tauri 2 system prerequisites](https://v2.tauri.app/start/prerequisites/) for
  your platform.
- [Node.js](https://nodejs.org/) 20+ and npm.
- [Docker](https://docs.docker.com/get-docker/) with Compose — only needed to
  work on the Context Engine.
- A Go toolchain (1.25+) — only needed to work on the Context Engine outside of
  Docker.

## Building and running

**Desktop app**

```bash
cd apps/desktop
npm install
npm run tauri:dev      # run the app in development
npm run tauri:build    # produce a release bundle
```

**Agent core / git-ops (Rust)**

```bash
cargo build                    # whole workspace
cargo test -p agent            # core unit + integration tests
cargo test -p git-ops          # checkpoint tests
```

Live LLM round-trip tests are marked `#[ignore]` and require a real API key in
the environment; run them explicitly with `cargo test -p agent -- --ignored`.
Don't commit keys — pass them via environment variables only.

**Context Engine (Go service)**

```bash
cd services/context-engine
cp .env.example .env           # set SUPERCODER_OPENAI_API_KEY
docker compose up -d --build
```

See [`services/context-engine/README.md`](./services/context-engine/README.md)
for health checks and wiring it to the app.

## Conventions

- **Follow the surrounding code.** Match existing naming, structure, and idioms
  in the file/crate you're editing rather than introducing new patterns.
- **Keep changes surgical.** Prefer small, focused PRs that do one thing.
- **Rust:** keep the core free of app/UI/network-framework coupling; new tools
  and providers belong behind the existing trait/enum seams.
- **Commits:** we use [Conventional Commits](https://www.conventionalcommits.org/)
  prefixes (`feat:`, `fix:`, `chore:`, `docs:`, …), optionally scoped
  (`feat(agent): …`).
- **Tests:** add or update tests for behavior you change. Rust changes should
  keep `cargo test -p agent -p git-ops` green; frontend changes run under
  `npm run test` (vitest) in `apps/desktop`.
- **Don't touch `v1/`.** It's frozen history.

## Pull requests

1. Fork and branch from `main` (or the active development branch).
2. Make your change, with tests, and ensure the relevant build/test commands
   above pass.
3. Open a PR and fill out the [pull request template](./.github/PULL_REQUEST_TEMPLATE.md):
   a clear description, related issue, and how you tested it.
4. Keep the PR atomic and focused on a single change.

A maintainer will review. Thanks for helping make SuperCoder better.
