/** Strip "group_" prefix from IDs (backend sometimes returns prefixed IDs) */
export function normalizeId(id: string | number): string {
  return String(id).replace(/^group_/, '');
}

/** Generate a temporary ID for optimistic messages */
export function generateTempId(): string {
  return `temp-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}
