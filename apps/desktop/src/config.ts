// ============================================================
// Configuration
// ============================================================

/** LLM gateway base URL. Override with VITE_GATEWAY_URL; defaults to the local
 * gateway. Any OpenAI-compatible endpoint works — set a custom one in Settings. */
export const GATEWAY_URL =
  import.meta.env.VITE_GATEWAY_URL ?? "http://localhost:8080/api/v1";

// Timeouts (ms)
export const HTTP_TIMEOUT = 10_000;
