// ============================================================
// Backend-aligned TypeScript interfaces
// ============================================================

// --- User ---

export interface UserProfile {
  id: number;
  first_name: string;
  last_name: string;
  email: string;
  avatar_url?: string;
  displayName: string;
  initials: string;
  avatarColor: string;
}

// --- Attachments ---

export interface Attachment {
  url: string;
  file_name: string;
  media_type: string;
}

// --- Messages ---

export interface Message {
  id: string;
  workspace_id: number;
  recipient_type: number; // 0=group, 1=DM, 2=groupDM
  recipient_id: number;
  thread_id: string;
  user_id: number;
  text: string;
  attachments: Attachment[];
  mentions: number[];
  mention_groups: number[];
  reactions: Record<string, number>; // { "👍": 3 }
  my_reactions: Record<string, boolean>; // { "👍": true }
  pinned: boolean;
  task_id: string;
  reply_count: number;
  broadcast_from_thread_id: string | null;
  also_sent_to_channel: boolean;
  sent_at: string;
  updated_at: string;
  edited: boolean;
  edited_at: string | null;
  is_system?: boolean;
  is_forwarded?: boolean;
  // Frontend-enriched
  sender?: UserProfile;
  _optimistic?: boolean;
  _tempId?: string;
  _agentThinking?: {
    toolCalls: { toolCallId: string; toolName: string; argsSummary: string; status: string; summary?: string }[];
    durationSeconds: number;
  };
  _collapsedCount?: number;
  _collapsedIds?: string[];
}

// --- Groups / Conversations ---

export interface GroupMember {
  id: number;
  group_id: number;
  user_id: number;
  role: 'ADMIN' | 'MODERATOR' | 'MEMBER';
  user?: UserProfile;
}

export interface Group {
  id: string;
  workspace_id: number;
  name: string;
  description: string;
  avatar_url: string;
  accessibility: 'private' | 'public' | 'dm' | 'group_dm' | 'self_dm';
  is_dm: boolean;
  recipient_type: number;
  recipient_id: number;
  other_user: UserProfile | null;
  unread_count: number;
  last_message: string;
  last_message_sender?: string;
  last_message_time: string;
  member_count?: number;
  is_member?: boolean;
  created_at: string;
  updated_at: string;
  _resolvedName?: string;
  _otherUserId?: string; // For DMs: the other member's user_id (matches message.user_id)
  _hasDraft?: boolean;
}

// --- Pagination ---

export interface Pagination {
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
}

// --- MQTT ---

export interface MqttCredentials {
  username: string;
  password: string;
  url: string;
  user_id?: number;
}

export type MqttEventType =
  | 'NEW_MESSAGE'
  | 'MESSAGE_EDITED'
  | 'MESSAGE_DELETED'
  | 'REACTION_UPDATED'
  | 'THREAD_COUNT_UPDATED'
  | 'NEW_THREAD_REPLY'
  | 'THREAD_REPLY_EDITED'
  | 'THREAD_REPLY_DELETED'
  | 'THREAD_REACTION_UPDATED'
  | 'TYPING'
  | 'SIDEBAR_NEW_MESSAGE'
  | 'GROUP_UPDATED'
  | 'ADDED_TO_GROUP'
  | 'REMOVED_FROM_GROUP'
  | 'UNREAD_UPDATE'
  // Huddle events
  | 'HUDDLE_STARTED'
  | 'HUDDLE_ENDED'
  | 'HUDDLE_JOINED'
  | 'HUDDLE_LEFT'
  | 'HUDDLE_MUTED'
  | 'HUDDLE_UNMUTED'
  | 'HUDDLE_ACTIVE_SPEAKER';

export interface MqttEventBase {
  event: MqttEventType;
  event_id: string;
  ts: number;
}

export interface MqttNewMessageEvent extends MqttEventBase {
  event: 'NEW_MESSAGE';
  group_id: string;
  message_id: string;
  thread_id: string;
  user_id: number;
}

export interface MqttMessageEditedEvent extends MqttEventBase {
  event: 'MESSAGE_EDITED';
  group_id: string;
  message_id: string;
}

export interface MqttMessageDeletedEvent extends MqttEventBase {
  event: 'MESSAGE_DELETED';
  group_id: string;
  message_id: string;
}

export interface MqttReactionEvent extends MqttEventBase {
  event: 'REACTION_UPDATED' | 'THREAD_REACTION_UPDATED';
  group_id: string;
  message_id: string;
  emoji_key: string;
  user_id: number;
  action: 'add' | 'remove';
}

export interface MqttThreadCountEvent extends MqttEventBase {
  event: 'THREAD_COUNT_UPDATED';
  group_id: string;
  thread_id: string;
  reply_count: number;
}

export interface MqttThreadReplyEvent extends MqttEventBase {
  event: 'NEW_THREAD_REPLY' | 'THREAD_REPLY_EDITED' | 'THREAD_REPLY_DELETED';
  group_id: string;
  thread_id: string;
  message_id: string;
  user_id?: number;
}

export interface MqttTypingEvent extends MqttEventBase {
  event: 'TYPING';
  group_id: string;
  user_id: number;
  is_typing: boolean;
}

export interface MqttSidebarNewMessageEvent extends MqttEventBase {
  event: 'SIDEBAR_NEW_MESSAGE';
  group_id: string;
  message_id: string;
  message_preview: string;
  sender_id: number;
}

export interface MqttGroupUpdatedEvent extends MqttEventBase {
  event: 'GROUP_UPDATED';
  group_id: string;
  name?: string;
  avatar_url?: string;
}

export interface MqttGroupMembershipEvent extends MqttEventBase {
  event: 'ADDED_TO_GROUP' | 'REMOVED_FROM_GROUP';
  group_id: string;
}

export interface MqttUnreadEvent extends MqttEventBase {
  event: 'UNREAD_UPDATE';
  group_id: string;
  unread_count: number;
}

export type MqttEvent =
  | MqttNewMessageEvent
  | MqttMessageEditedEvent
  | MqttMessageDeletedEvent
  | MqttReactionEvent
  | MqttThreadCountEvent
  | MqttThreadReplyEvent
  | MqttTypingEvent
  | MqttSidebarNewMessageEvent
  | MqttGroupUpdatedEvent
  | MqttGroupMembershipEvent
  | MqttUnreadEvent;

// --- API Request Payloads ---

export interface SendMessagePayload {
  recipient_type: number;
  recipient_id?: number;
  text: string;
  thread_id?: string;
  attachments?: Attachment[];
  mentions?: number[];
  mention_groups?: number[];
  also_send_to_channel?: boolean;
  user_ids?: number[];
  group_name?: string;
  is_forwarded?: boolean;
}

export interface GetGroupsParams {
  type?: 'all' | 'groups' | 'unread' | 'dms';
  search?: string;
  limit?: number;
  offset?: number;
}

export interface GetMessagesParams {
  group_id: string;
  limit?: number;
  offset?: number;
  search?: string;
}

export interface GetThreadRepliesParams {
  group_id: string;
  thread_id: string;
  limit?: number;
  offset?: number;
}

export interface CreateGroupPayload {
  name: string;
  description?: string;
  type: 'public' | 'private';
  members: { user_id: number; role: 'ADMIN' | 'MEMBER' }[];
}
