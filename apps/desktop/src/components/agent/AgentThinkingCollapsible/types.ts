export interface ToolCallItem {
  toolCallId: string;
  toolName: string;
  argsSummary: string;
  status: string;
  summary?: string;
}

export interface StreamingProps {
  startedAt: number;
  isThinking: boolean;
  toolCalls: ToolCallItem[];
  durationSeconds?: never;
}

export interface FinalizedProps {
  durationSeconds: number;
  toolCalls: ToolCallItem[];
  startedAt?: never;
  isThinking?: never;
}

export type AgentThinkingCollapsibleProps = StreamingProps | FinalizedProps;
