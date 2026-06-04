# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities **privately** — do not open a public
issue, discussion, or pull request for a suspected vulnerability.

Use GitHub's private vulnerability reporting:

1. Go to the repository's **Security** tab.
2. Click **Report a vulnerability** (under "Advisories").
3. Provide a description, affected component, reproduction steps, and impact.

We'll acknowledge your report, investigate, and keep you updated on a fix and
disclosure timeline.

## Scope

SuperCoder is a local-first desktop application. Useful things to include in a
report:

- The component: agent core (`crates/agent`), git-ops, the desktop bridge
  (`apps/desktop/src-tauri`), or the Context Engine (`services/context-engine`).
- Whether it's reachable in the default zero-backend configuration or only with
  the optional Context Engine enabled.
- The trust boundary crossed (e.g. path traversal outside the project,
  unexpected outbound requests, command execution).

The legacy `v1/` tree is frozen and unmaintained; please do not report issues
against it.

## Handling secrets

Never include real API keys, tokens, or other credentials in a report, issue, or
PR. Configure provider keys only via the app's Settings or environment variables.

## Security model notes

SuperCoder is a single-user, local-first desktop app. A few behaviors are
intentional given that model — they assume an attacker who can already run code
as your user has no boundary left to cross, so they are not treated as
vulnerabilities:

- **Context-engine embedding key in the process environment.** In app-managed
  mode the embedding key is passed to the local Docker stack via the
  `SUPERCODER_OPENAI_API_KEY` environment variable. Same-UID processes can read
  it (e.g. `/proc/<pid>/environ`), as they can already read the app's local
  SQLite store. The key is never logged or written into the compose file.
- **`SUPERCODER_CE_*` environment overrides** (compose-file path, image refs,
  mode, port) are trusted developer conveniences for local testing. Setting them
  requires control of the launch environment, which already implies user-level
  code execution.
- **User-mode backend URL probe.** In user mode the app probes the backend URL
  *you* configure (`/api/health`) with no host allowlist — that is the feature
  (connect to your own self-run engine), not an SSRF sink.
