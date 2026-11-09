import { withAuth } from "@/lib/config";

export interface NewsletterSubscriber {
  email: string;
  name: string;
  source: string;
  createdAt: string;
}

function subscribersUrl(base: string, workspaceId: string): string {
  return `${base}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/newsletter/subscribers`;
}

async function errorMessage(response: Response, fallback: string): Promise<string> {
  const body = await response.json().catch(() => null);
  return typeof body?.error === "string" ? body.error : fallback;
}

export async function listNewsletterSubscribers(
  base: string,
  workspaceId: string,
): Promise<NewsletterSubscriber[]> {
  const response = await fetch(subscribersUrl(base, workspaceId), { headers: withAuth() });
  if (!response.ok) throw new Error("Could not load subscribers.");
  const data: unknown = await response.json();
  if (
    !data ||
    typeof data !== "object" ||
    !("subscribers" in data) ||
    !Array.isArray(data.subscribers) ||
    !data.subscribers.every(
      (s: unknown) =>
        typeof s === "object" &&
        s !== null &&
        "email" in s &&
        typeof s.email === "string" &&
        "name" in s &&
        typeof s.name === "string" &&
        "source" in s &&
        typeof s.source === "string" &&
        "createdAt" in s &&
        typeof s.createdAt === "string",
    )
  )
    throw new Error("Invalid subscribers response.");
  return data.subscribers as NewsletterSubscriber[];
}

export async function addNewsletterSubscriber(
  base: string,
  workspaceId: string,
  email: string,
  name: string,
): Promise<void> {
  const response = await fetch(subscribersUrl(base, workspaceId), {
    method: "POST",
    headers: withAuth({ "Content-Type": "application/json" }),
    body: JSON.stringify({ email, name }),
  });
  if (!response.ok) {
    throw new Error(await errorMessage(response, "Could not add subscriber."));
  }
}

export async function removeNewsletterSubscriber(
  base: string,
  workspaceId: string,
  email: string,
): Promise<void> {
  const response = await fetch(
    `${subscribersUrl(base, workspaceId)}?email=${encodeURIComponent(email)}`,
    { method: "DELETE", headers: withAuth() },
  );
  if (!response.ok && response.status !== 404) {
    throw new Error(await errorMessage(response, "Could not remove subscriber."));
  }
}
