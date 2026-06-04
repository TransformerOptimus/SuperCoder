export interface AgentInputProps {
  /** The session this composer sends into. */
  sessionId: string;
  /** The session's project folder (used for @file/skills/subagents lookup). */
  folderPath: string | null;
  /** Display name for the placeholder. */
  agentName?: string;
}
