import { create } from 'zustand';
import { createAgentSlice } from './agentSlice';
import { createAgentChatSlice } from './agentChatSlice';
import type { AppStore } from './types';

export type { AppStore };

export const useAppStore = create<AppStore>()((...a) => ({
  ...createAgentSlice(...a),
  ...createAgentChatSlice(...a),
}));
