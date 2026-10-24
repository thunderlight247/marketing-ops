export const DEFAULT_API_BASE_URL = "http://localhost:8080";

/**
 * Resolves the API base URL from NEXT_PUBLIC_API_BASE_URL.
 * The argument exists so tests can pass a value; callers normally omit it.
 * The literal `process.env.NEXT_PUBLIC_API_BASE_URL` access must stay in this
 * default expression so Next.js can inline it at build time.
 */
export function getApiBaseUrl(
  raw: string | undefined = process.env.NEXT_PUBLIC_API_BASE_URL,
): string {
  const value = (raw ?? "").trim();
  if (value === "") return DEFAULT_API_BASE_URL;
  return value.replace(/\/+$/, "");
}

/**
 * Returns the Authorization header value for every API request, or undefined
 * when no API key is configured (development mode — auth is disabled server-side
 * too in that case).
 *
 * NEXT_PUBLIC_API_KEY is the client-side bearer token. It is public in the
 * sense that the browser must hold it to make authenticated requests, but it
 * must never be committed to source control or hardcoded here.
 */
export function getApiAuthHeader(
  raw: string | undefined = process.env.NEXT_PUBLIC_API_KEY,
): string | undefined {
  const value = (raw ?? "").trim();
  if (value === "") return undefined;
  return `Bearer ${value}`;
}

/**
 * Merges the Authorization header into a Headers-compatible object.
 * Returns the merged headers so call sites can spread them into RequestInit.
 */
export function withAuth(
  existing: Record<string, string> = {},
): Record<string, string> {
  const auth = getApiAuthHeader();
  if (auth === undefined) return existing;
  return { ...existing, Authorization: auth };
}
