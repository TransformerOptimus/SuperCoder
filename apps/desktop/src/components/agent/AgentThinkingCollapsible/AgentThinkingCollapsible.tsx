import { useMemo } from "react";
import { Collapse } from "antd";
import { Loader2, Check, X } from "lucide-react";
import type { AgentThinkingCollapsibleProps } from "./types";

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds} seconds`;
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  if (mins < 60) {
    return secs > 0 ? `${mins}m ${secs}s` : `${mins} minutes`;
  }
  const hrs = Math.floor(mins / 60);
  const remainMins = mins % 60;
  return remainMins > 0 ? `${hrs}h ${remainMins}m` : `${hrs} hours`;
}

function cleanArgsSummary(toolName: string, raw: string): string {
  // Persisted subagent thinking step — fallback called cleanArgsSummary(toolName, toolName).
  // Chip label already shows "Subagent: <name>"; return empty to avoid the
  // duplicated "Subagent: code-explorer Subagent: code-explorer" render.
  if (raw === toolName && toolName.startsWith("Subagent: ")) {
    return "";
  }
  const trimmed = raw.trim();
  if (trimmed.startsWith("{") || trimmed.startsWith("[") || trimmed.startsWith('"')) {
    // TodoWrite — extract first todo content via regex (JSON is often truncated)
    if (toolName === "TodoWrite" || trimmed.includes('"todos"')) {
      const match = trimmed.match(/"content"\s*:\s*"([^"]+)"/);
      if (match) return `Updating todos: ${match[1]}`;
      return "Updating todos";
    }
    if (toolName === "TodoRead") return "Reading todos";
    // codebase_search / codebase_graph — extract "query" from JSON args
    if (toolName === "codebase_search" || toolName === "codebase_graph") {
      const match = trimmed.match(/"query"\s*:\s*"([^"]+)"/);
      if (match) return match[1];
    }
    return toolName.startsWith("{") ? "Processing..." : toolName;
  }
  return raw;
}

const TOOL_COLORS: Record<string, { text: string; bg: string }> = {
  Read:     { text: '#3b82f6', bg: 'rgba(59,130,246,0.12)' },
  Glob:     { text: '#3b82f6', bg: 'rgba(59,130,246,0.12)' },
  Grep:     { text: '#3b82f6', bg: 'rgba(59,130,246,0.12)' },
  Search:   { text: '#3b82f6', bg: 'rgba(59,130,246,0.12)' },
  codebase_search: { text: '#06b6d4', bg: 'rgba(6,182,212,0.12)' },
  codebase_graph:  { text: '#06b6d4', bg: 'rgba(6,182,212,0.12)' },
  Writ:     { text: '#f59e0b', bg: 'rgba(245,158,11,0.12)' },
  Edit:     { text: '#f59e0b', bg: 'rgba(245,158,11,0.12)' },
  Bash:     { text: '#22c55e', bg: 'rgba(34,197,94,0.12)' },
  Run:      { text: '#22c55e', bg: 'rgba(34,197,94,0.12)' },
  Check:    { text: '#22c55e', bg: 'rgba(34,197,94,0.12)' },
  Review:   { text: '#22c55e', bg: 'rgba(34,197,94,0.12)' },
  Show:     { text: '#22c55e', bg: 'rgba(34,197,94,0.12)' },
  Todo:     { text: '#a855f7', bg: 'rgba(168,85,247,0.12)' },
  Updating: { text: '#a855f7', bg: 'rgba(168,85,247,0.12)' },
  Sav:      { text: '#ec4899', bg: 'rgba(236,72,153,0.12)' },
  Saving:   { text: '#ec4899', bg: 'rgba(236,72,153,0.12)' },
  skill:    { text: '#0891b2', bg: 'rgba(8,145,178,0.12)' },
  subagent: { text: '#a855f7', bg: 'rgba(168,85,247,0.12)' },
};

const DEFAULT_TAG_STYLE = { text: 'var(--text-secondary)', bg: 'var(--bg-secondary)' };

// Friendly display labels for snake_case tool names
const TOOL_LABELS: Record<string, string> = {
  codebase_search: 'Semantic search',
  codebase_graph: 'Querying graph',
  skill: 'Skill',
};

// Multi-word prefix matchers — used when toolName comes from persisted thinking
// text (e.g. "Semantic search: main entry point...") instead of structured live
// events. Order matters: more specific prefixes must come before generic ones
// (e.g. "Reading todos" before "Reading"). Both current and legacy phrasings
// are listed so old persisted messages keep their colored chips.
const PREFIX_MATCHERS: Array<{ prefix: string; label: string; colorKey: string }> = [
  // codebase tools (cyan)
  { prefix: 'Semantic search',    label: 'Semantic search', colorKey: 'codebase_search' },
  { prefix: 'Searching codebase', label: 'Semantic search', colorKey: 'codebase_search' }, // legacy
  { prefix: 'Querying graph',     label: 'Querying graph',  colorKey: 'codebase_graph' },
  // skills (teal)
  { prefix: 'Loading skill',      label: 'Loading skill',   colorKey: 'skill' },
  // todos (purple) — must precede "Reading"/"Updating"
  { prefix: 'Reading todos',      label: 'Reading todos',   colorKey: 'Todo' },
  { prefix: 'Updating todos',     label: 'Updating todos',  colorKey: 'Todo' },
  // plan (pink/orange)
  { prefix: 'Saving plan',        label: 'Saving plan',     colorKey: 'Sav' },
  { prefix: 'Editing plan',       label: 'Editing plan',    colorKey: 'Edit' },
  // file ops
  { prefix: 'Reading',            label: 'Reading',         colorKey: 'Read' },
  { prefix: 'Writing',            label: 'Writing',         colorKey: 'Writ' },
  { prefix: 'Editing',            label: 'Editing',         colorKey: 'Edit' },
  { prefix: 'Searching for',      label: 'Searching',       colorKey: 'Search' },
  // session/PR
  { prefix: 'Starting session',   label: 'Starting',        colorKey: 'Run' },
  { prefix: 'Creating PR',        label: 'Creating PR',     colorKey: 'Run' },
];

/** Extract short label + tag style from toolName (which may be "Reading services/foo.go") */
function getToolLabel(toolName: string): { label: string; style: { text: string; bg: string } } {
  if (toolName.startsWith("{") || toolName.startsWith("[")) {
    return { label: "Tool", style: TOOL_COLORS.Todo };
  }
  // "Subagent: <name>" — used for BOTH live events (via agent:subagent_start
  // → addToolCall with toolName="Subagent: <name>") and persisted thinking
  // markers (step text baked in by the Rust relay at SubagentStart). Both
  // paths share this format so the chip renders identically.
  if (toolName.startsWith("Subagent: ")) {
    const agentName = toolName.slice("Subagent: ".length);
    return { label: `Subagent: ${agentName}`, style: TOOL_COLORS.subagent };
  }
  // Direct match (e.g. "Read", "Edit", "Bash", "codebase_search") — live streaming
  if (TOOL_COLORS[toolName]) {
    const label = TOOL_LABELS[toolName] ?? toolName;
    return { label, style: TOOL_COLORS[toolName] };
  }
  // Multi-word prefix match — persisted thinking text path
  for (const { prefix, label, colorKey } of PREFIX_MATCHERS) {
    if (toolName.startsWith(prefix)) {
      return { label, style: TOOL_COLORS[colorKey] ?? DEFAULT_TAG_STYLE };
    }
  }
  // Last-resort first-word fallback (e.g. "Reading" → match "Read")
  const firstWord = toolName.split(/\s/)[0];
  const key = Object.keys(TOOL_COLORS).find(
    (k) => firstWord.toLowerCase().startsWith(k.toLowerCase()),
  );
  if (key) return { label: firstWord, style: TOOL_COLORS[key] };
  return { label: firstWord, style: DEFAULT_TAG_STYLE };
}

const STATUS_ICONS: Record<string, React.ReactNode> = {
  running: (
    <Loader2 className="w-3 h-3 animate-spin opacity-60 text-[var(--text-secondary)] shrink-0" />
  ),
  success: (
    <Check className="w-3 h-3 opacity-60 text-[var(--text-secondary)] shrink-0" />
  ),
  error: (
    <X className="w-3 h-3 opacity-60 text-[var(--text-secondary)] shrink-0" />
  ),
};

export default function AgentThinkingCollapsible(
  props: AgentThinkingCollapsibleProps,
) {
  const { toolCalls } = props;

  const elapsedSeconds = useMemo(() => {
    if ("durationSeconds" in props && props.durationSeconds !== undefined) {
      return props.durationSeconds;
    }
    if (props.isThinking) return 0;
    return Math.round((Date.now() - props.startedAt) / 1000);
  }, [props]);

  if (toolCalls.length === 0) return null;

  return (
    <div className="mb-1.5">
      <Collapse
        ghost
        size="small"
        items={[
          {
            key: "thinking",
            label: (
              <span className="text-xs text-[var(--text-secondary)]">
                Thought for {formatDuration(elapsedSeconds)}
              </span>
            ),
            children: (
              <div className="space-y-1.5 overflow-hidden">
                {toolCalls.map((tc) => {
                  const { label, style: tagStyle } = getToolLabel(tc.toolName);
                  const summary = tc.argsSummary
                    ? cleanArgsSummary(tc.toolName, tc.argsSummary)
                    : cleanArgsSummary(tc.toolName, tc.toolName);
                  return (
                    <div
                      key={tc.toolCallId}
                      className="flex items-center gap-2 text-xs text-[var(--text-secondary)] min-w-0"
                    >
                      {STATUS_ICONS[tc.status] ?? STATUS_ICONS.error}
                      <span
                        className="text-[10px] leading-4 px-1.5 py-0 shrink-0 rounded font-medium"
                        style={{
                          color: tagStyle.text,
                          background: tagStyle.bg,
                        }}
                      >
                        {label}
                      </span>
                      <span className="truncate">{summary}</span>
                      {tc.summary && (
                        <span className="ml-auto truncate opacity-60">
                          {tc.summary}
                        </span>
                      )}
                    </div>
                  );
                })}
              </div>
            ),
          },
        ]}
      />
    </div>
  );
}
