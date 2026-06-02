import { useState } from "react";
import { open as shellOpen } from "@tauri-apps/plugin-shell";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import AgentThinkingCollapsible from "../AgentThinkingCollapsible/AgentThinkingCollapsible";
import RewindDropdown, { type RewindAgentProps } from "../AgentThreadPanel/RewindDropdown";
import type { AgentMessage } from "@/types/agent";
import thinkingStyles from "../AgentThinking/AgentThinking.module.css";

interface Props {
  message: AgentMessage;
  onRewindAgent?: RewindAgentProps;
}

export default function AgentMessageBubble({ message, onRewindAgent }: Props) {
  const [hovered, setHovered] = useState(false);
  const isUser = message.role === "user";
  const isStreaming = message.id.endsWith("-streaming");

  // ── Streaming placeholders (agent "thinking") ──────────────────────────
  if (message.text === "__thinking__") {
    return (
      <div className="py-1.5">
        <span className={thinkingStyles.thinking_text}>Thinking</span>
      </div>
    );
  }
  if (message.text?.toLowerCase().startsWith("__thinking_subagent:")) {
    const name = message.text.slice("__thinking_subagent:".length, -2).trim() || "...";
    return (
      <div className="py-1.5">
        <span className={thinkingStyles.thinking_text}>Running subagent {name}</span>
      </div>
    );
  }

  // ── User → right-aligned bubble (iMessage style) ───────────────────────
  if (isUser) {
    return (
      <div
        className="group flex justify-end items-center gap-2 py-1"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        {hovered && onRewindAgent && (
          <div className="shrink-0">
            <RewindDropdown {...onRewindAgent} />
          </div>
        )}
        <div className="max-w-[78%] rounded-2xl bg-gray-100 dark:bg-[var(--bg-secondary)] px-3.5 py-2 text-sm text-gray-800 dark:text-gray-100 whitespace-pre-wrap break-words">
          {message.text}
        </div>
      </div>
    );
  }

  // ── Agent → plain left-aligned text (no avatar / name) ─────────────────
  // Thinking chips come from message.thinking — set live from streaming tool
  // events, and reconstructed from the SQLite tool_call rows on reload.
  const thinkingToolCalls = message.thinking?.toolCalls ?? [];
  const thinkingDuration = message.thinking?.durationSeconds ?? 0;
  const bodyText = message.text;

  return (
    <div className="py-1.5">
      {thinkingToolCalls.length > 0 && (
        <AgentThinkingCollapsible durationSeconds={thinkingDuration} toolCalls={thinkingToolCalls} />
      )}
      {bodyText && (
        <div
          className={`message-html text-sm leading-relaxed text-gray-800 dark:text-gray-200 overflow-x-auto${isStreaming ? " streaming-text" : ""}`}
        >
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              a: ({ href, children }) => (
                <a
                  href={href}
                  className="text-blue-600 dark:text-blue-400 underline hover:text-blue-800 dark:hover:text-blue-300"
                  onClick={(e) => {
                    e.preventDefault();
                    if (href) shellOpen(href).catch(() => window.open(href, "_blank"));
                  }}
                >
                  {children}
                </a>
              ),
            }}
          >
            {bodyText}
          </ReactMarkdown>
        </div>
      )}
    </div>
  );
}
