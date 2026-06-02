const DRAFT_PREFIX = 'chat_draft_';

export interface Draft {
  text: string;
  mentionedUserIds: number[];
  files?: string[];
  timestamp: number;
}

export function saveDraft(chatId: string, text: string, mentionedUserIds: number[] = [], files: string[] = []): void {
  const trimmed = text.trim();
  const hasFiles = files.length > 0;
  if (!trimmed && !hasFiles) {
    localStorage.removeItem(`${DRAFT_PREFIX}${chatId}`);
    window.dispatchEvent(new Event('draft-updated'));
    return;
  }
  const draft: Draft = { text, mentionedUserIds, ...(hasFiles && { files }), timestamp: Date.now() };
  localStorage.setItem(`${DRAFT_PREFIX}${chatId}`, JSON.stringify(draft));
  window.dispatchEvent(new Event('draft-updated'));
}

export function loadDraft(chatId: string): Draft | null {
  try {
    const raw = localStorage.getItem(`${DRAFT_PREFIX}${chatId}`);
    if (!raw) return null;
    return JSON.parse(raw) as Draft;
  } catch {
    return null;
  }
}

export function clearDraft(chatId: string): void {
  localStorage.removeItem(`${DRAFT_PREFIX}${chatId}`);
  window.dispatchEvent(new Event('draft-updated'));
}

export function getDraftChatIds(): Set<string> {
  const ids = new Set<string>();
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key?.startsWith(DRAFT_PREFIX)) {
      const chatId = key.slice(DRAFT_PREFIX.length);
      try {
        const draft = JSON.parse(localStorage.getItem(key) || '{}') as Draft;
        if (draft.text?.trim()) {
          ids.add(chatId);
        }
      } catch {
        // skip malformed
      }
    }
  }
  return ids;
}

export function getDraftPreview(chatId: string): string | null {
  const draft = loadDraft(chatId);
  if (!draft?.text?.trim()) return null;
  const text = draft.text.trim();
  // Strip HTML tags if draft contains rich text
  const plain = /<[a-z][\s\S]*>/i.test(text)
    ? text.replace(/<[^>]*>/g, '').trim()
    : text;
  if (!plain) return null;
  return plain.replace(/\n/g, ' ');
}
