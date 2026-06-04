import type { AgentSlice } from './agentSlice';
import type { AgentChatSlice } from './agentChatSlice';

export type AppStore = AgentSlice & AgentChatSlice;
