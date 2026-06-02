import type { Agent } from '../../../types/agent';

export interface AgentConversationItemProps {
  agent: Agent;
  isActive: boolean;
  onClick: () => void;
}
