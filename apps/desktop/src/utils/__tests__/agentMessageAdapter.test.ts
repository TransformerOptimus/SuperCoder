import { describe, it, expect, vi } from 'vitest';
import {
  buildAgentMessage,
  buildChatPanelMessage,
  buildThinkingMeta,
  agentMessageToMessage,
  parseThinkingMarkers,
  adaptAgentMessages,
} from '../agentMessageAdapter';
import type { AgentToolCallState } from '@/types/agent';

// Mock avatarUtils — these are visual-only helpers
vi.mock('@/utils/avatarUtils', () => ({
  getAvatarColor: (id: string) => `color-${id}`,
  nameToInitials: (name: string) => name.charAt(0).toUpperCase(),
}));

const agentProfile = { id: 42, name: 'TestAgent', first_name: 'Test', last_name: 'Agent', email: 'agent@test.com' };
const userProfile = { id: 1, first_name: 'Alice', last_name: 'Smith', email: 'alice@test.com', displayName: 'Alice Smith', initials: 'AS', avatarColor: 'blue' };

describe('buildAgentMessage', () => {
  it('builds a user-role message with defaults', () => {
    const msg = buildAgentMessage('msg-1', 'hello', 'user', 'thread-1', 'agent-42');
    expect(msg.id).toBe('msg-1');
    expect(msg.text).toBe('hello');
    expect(msg.role).toBe('user');
    expect(msg.thread_id).toBe('thread-1');
    expect(msg.agent_id).toBe('agent-42');
    expect(msg.artifacts).toEqual([]);
    expect(msg.created_at).toBeTruthy();
  });

  it('builds an agent-role message with artifacts', () => {
    const artifact = {
      id: 'art-1', type: 'text' as const, name: 'note', created_at: '', content: 'hi', format: 'plain' as const,
    };
    const msg = buildAgentMessage('msg-2', 'done', 'agent', 'thread-2', 'agent-42', [artifact]);
    expect(msg.role).toBe('agent');
    expect(msg.artifacts).toHaveLength(1);
    expect(msg.artifacts[0].id).toBe('art-1');
  });
});

describe('buildChatPanelMessage', () => {
  it('builds a basic Message with agent sender', () => {
    const msg = buildChatPanelMessage('msg-1', 'hello world', agentProfile);
    expect(msg.id).toBe('msg-1');
    expect(msg.text).toBe('hello world');
    expect(msg.user_id).toBe(42);
    expect(msg.sender?.displayName).toBe('TestAgent');
    expect(msg.recipient_type).toBe(1);
    expect(msg._agentThinking).toBeUndefined();
  });

  it('attaches thinking metadata when provided', () => {
    const thinking = {
      toolCalls: [{ toolCallId: 't1', toolName: 'read', argsSummary: 'file.ts', status: 'success' }],
      durationSeconds: 5,
    };
    const msg = buildChatPanelMessage('msg-2', 'text', agentProfile, thinking);
    expect(msg._agentThinking).toBeDefined();
    expect(msg._agentThinking!.toolCalls).toHaveLength(1);
    expect(msg._agentThinking!.durationSeconds).toBe(5);
  });

  it('applies overrides', () => {
    const msg = buildChatPanelMessage('msg-3', 'session', agentProfile, undefined, { reply_count: 1 });
    expect(msg.reply_count).toBe(1);
  });
});

describe('buildThinkingMeta', () => {
  it('returns metadata from parsed markers', () => {
    const result = buildThinkingMeta(['Read file.ts', 'Edit code'], 12);
    expect(result).toBeDefined();
    expect(result!.toolCalls).toHaveLength(2);
    expect(result!.toolCalls[0].toolName).toBe('Read file.ts');
    expect(result!.durationSeconds).toBe(12);
  });

  it('falls back to tool calls when no parsed steps', () => {
    const toolCalls: AgentToolCallState[] = [
      { toolCallId: 'tc-1', toolName: 'bash', argsSummary: 'ls', status: 'success' },
    ];
    const now = Date.now();
    const result = buildThinkingMeta([], 0, toolCalls, now - 5000);
    expect(result).toBeDefined();
    expect(result!.toolCalls).toHaveLength(1);
    expect(result!.durationSeconds).toBeGreaterThanOrEqual(4);
    expect(result!.durationSeconds).toBeLessThanOrEqual(6);
  });

  it('returns undefined when no data', () => {
    expect(buildThinkingMeta([], 0)).toBeUndefined();
    expect(buildThinkingMeta([], 0, [], null)).toBeUndefined();
  });
});

describe('agentMessageToMessage', () => {
  it('converts user-role AgentMessage', () => {
    const agentMsg = {
      id: 'am-1', thread_id: 'thread-1', agent_id: 'a42', role: 'user' as const,
      text: 'hi', artifacts: [], created_at: '2026-01-01T00:00:00Z',
    };
    const msg = agentMessageToMessage(agentMsg, agentProfile, userProfile);
    expect(msg.user_id).toBe(1); // current user
    expect(msg.sender?.displayName).toBe('Alice Smith');
    expect(msg.sent_at).toBe('2026-01-01T00:00:00Z');
  });

  it('converts agent-role AgentMessage', () => {
    const agentMsg = {
      id: 'am-2', thread_id: 'thread-1', agent_id: 'a42', role: 'agent' as const,
      text: 'done', artifacts: [], created_at: '2026-01-01T00:00:00Z',
    };
    const msg = agentMessageToMessage(agentMsg, agentProfile, userProfile);
    expect(msg.user_id).toBe(42); // agent
    expect(msg.sender?.displayName).toBe('TestAgent');
  });

  it('handles null currentUser with fallback', () => {
    const agentMsg = {
      id: 'am-3', thread_id: 'thread-1', agent_id: 'a42', role: 'user' as const,
      text: 'hi', artifacts: [], created_at: '2026-01-01T00:00:00Z',
    };
    const msg = agentMessageToMessage(agentMsg, agentProfile, null);
    expect(msg.user_id).toBe(0);
    expect(msg.sender?.displayName).toBe('You');
  });
});

describe('parseThinkingMarkers', () => {
  it('parses markers from content', () => {
    const content = '<!-- thinking duration=10 -->\nRead file\nEdit code\n<!-- /thinking -->\nHere is the result.';
    const { cleanText, thinkingSteps, durationSeconds } = parseThinkingMarkers(content);
    expect(cleanText).toBe('Here is the result.');
    expect(thinkingSteps).toEqual(['Read file', 'Edit code']);
    expect(durationSeconds).toBe(10);
  });

  it('returns content unchanged when no markers', () => {
    const { cleanText, thinkingSteps, durationSeconds } = parseThinkingMarkers('plain text');
    expect(cleanText).toBe('plain text');
    expect(thinkingSteps).toEqual([]);
    expect(durationSeconds).toBe(0);
  });

  it('handles empty content', () => {
    const { cleanText, thinkingSteps } = parseThinkingMarkers('');
    expect(cleanText).toBe('');
    expect(thinkingSteps).toEqual([]);
  });

  it('parses multiple steps', () => {
    const content = '<!-- thinking duration=5 -->\nStep 1\nStep 2\nStep 3\n<!-- /thinking -->\nDone.';
    const { thinkingSteps } = parseThinkingMarkers(content);
    expect(thinkingSteps).toHaveLength(3);
  });

  it('handles multiple thinking blocks', () => {
    const content = '<!-- thinking duration=3 -->\nRead file\n<!-- /thinking -->\nFirst part.\n<!-- thinking duration=7 -->\nEdit code\nRun tests\n<!-- /thinking -->\nSecond part.';
    const { cleanText, thinkingSteps, durationSeconds } = parseThinkingMarkers(content);
    expect(thinkingSteps).toEqual(['Read file', 'Edit code', 'Run tests']);
    expect(durationSeconds).toBe(10); // 3 + 7
    expect(cleanText).not.toContain('<!-- thinking');
    expect(cleanText).toContain('First part.');
    expect(cleanText).toContain('Second part.');
  });
});

describe('adaptAgentMessages', () => {
  it('converts display messages to chat Messages', () => {
    const displayMsgs = [
      { id: '1', role: 'user' as const, text: 'hello', created_at: '2026-01-01T00:00:00Z', thread_id: '' },
      { id: '2', role: 'assistant' as const, text: 'hi back', created_at: '2026-01-01T00:00:01Z', thread_id: '' },
    ];
    const result = adaptAgentMessages(displayMsgs, 1, 'Alice', 42, 'TestAgent');
    expect(result).toHaveLength(2);
    expect(result[0].id).toBe('agent-db-1');
    expect(result[0].user_id).toBe(1);
    expect(result[1].user_id).toBe(42);
  });

  it('extracts thinking markers from display messages', () => {
    const displayMsgs = [
      { id: '3', role: 'assistant' as const, text: '<!-- thinking duration=8 -->\nDid stuff\n<!-- /thinking -->\nResult', created_at: '', thread_id: '' },
    ];
    const result = adaptAgentMessages(displayMsgs, 1, 'Alice', 42, 'TestAgent');
    expect(result[0]._agentThinking).toBeDefined();
    expect(result[0]._agentThinking!.durationSeconds).toBe(8);
    expect(result[0].text).toBe('Result');
  });
});
