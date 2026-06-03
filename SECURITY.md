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
