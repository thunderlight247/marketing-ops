import { getApiBaseUrl } from "@/lib/config";

export interface Operator {
  id: string;
  name: string;
  email: string;
  role: "admin" | "editor" | "viewer" | string;
  status: "active" | "inactive" | string;
  createdAt: string;
}

export async function fetchOperators(baseUrl = getApiBaseUrl()): Promise<Operator[]> {
  try {
    const res = await fetch(`${baseUrl}/api/v1/operators`, { cache: "no-store" });
    if (!res.ok) return [];
    return await res.json();
  } catch {
    return [];
  }
}

export async function createOperator(
  baseUrl = getApiBaseUrl(),
  input: { name: string; email: string; role: string },
): Promise<{ ok: boolean; operator?: Operator; error?: string }> {
  try {
    const res = await fetch(`${baseUrl}/api/v1/operators`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      return { ok: false, error: err.error || "Failed to create team member." };
    }
    const created = await res.json();
    return { ok: true, operator: created };
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : "Network error" };
  }
}

export async function updateOperator(
  baseUrl = getApiBaseUrl(),
  id: string,
  input: { role?: string; status?: string },
): Promise<{ ok: boolean; operator?: Operator; error?: string }> {
  try {
    const res = await fetch(`${baseUrl}/api/v1/operators/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      return { ok: false, error: err.error || "Failed to update team member." };
    }
    const updated = await res.json();
    return { ok: true, operator: updated };
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : "Network error" };
  }
}
