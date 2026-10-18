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
