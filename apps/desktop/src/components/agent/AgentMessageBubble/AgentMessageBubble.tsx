import { useState } from "react";
import Markdown from "../../common/Markdown";
import ImageLightbox from "./ImageLightbox";
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
  const [preview, setPreview] = useState<string | null>(null);
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
    const images = message.images ?? [];
    return (
      <div
        className="group flex justify-end items-start gap-2 py-1"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        {hovered && onRewindAgent && (
          <div className="shrink-0 mt-1.5">
            <RewindDropdown {...onRewindAgent} />
          </div>
        )}
        <div className="flex flex-col items-end gap-1.5 max-w-[78%]">
          {images.length > 0 && (
            <div className="flex flex-wrap justify-end gap-1.5">
              {images.map((src, i) => (
                <img
                  key={i}
                  src={src}
                  alt={`attachment ${i + 1}`}
                  onClick={() => setPreview(src)}
                  className="max-h-48 max-w-full rounded-xl border border-gray-200 dark:border-dark-border cursor-zoom-in"
                />
              ))}
            </div>
          )}
          {message.text && (
            <div className="rounded-2xl bg-gray-100 dark:bg-[var(--bg-secondary)] px-3.5 py-2 text-sm text-gray-800 dark:text-gray-100 whitespace-pre-wrap break-words">
              {message.text}
            </div>
          )}
        </div>
        {preview && <ImageLightbox src={preview} onClose={() => setPreview(null)} />}
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
        <Markdown
          className={`message-html text-sm leading-relaxed text-gray-800 dark:text-gray-200 overflow-x-auto${isStreaming ? " streaming-text" : ""}`}
        >
          {bodyText}
        </Markdown>
      )}
    </div>
  );
}
