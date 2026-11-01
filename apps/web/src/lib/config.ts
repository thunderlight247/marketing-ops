// Browser requests go through the same-origin server proxy. An explicit base
// remains available to unit tests and callers targeting an API directly.
export const DEFAULT_API_BASE_URL = "";

export function getApiBaseUrl(raw?: string): string {
  return (raw ?? "").trim().replace(/\/+$/, "");
}

// API authentication is added by the server proxy, never by browser code.
export function withAuth(existing: Record<string, string> = {}): Record<string, string> {
  return existing;
}
