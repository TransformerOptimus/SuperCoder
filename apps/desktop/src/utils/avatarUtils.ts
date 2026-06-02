const AVATAR_COLORS = ['#D97706', '#92400E', '#6D28D9', '#059669', '#2563EB', '#DC2626'];

/** Fixed color for the current (logged-in) user */
export const CURRENT_USER_COLOR = '#6B7280';

/** Deterministic color from any string key (user_id, group_id, etc.) */
export function getAvatarColor(key: string): string {
  let hash = 0;
  for (const ch of key) hash = (hash * 31 + ch.charCodeAt(0)) | 0;
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
}

/** Extract initials from a display name (first letter of each word, max 2) */
export function nameToInitials(name: string): string {
  return name.split(' ').map((w) => w[0]).join('').slice(0, 2).toUpperCase();
}
