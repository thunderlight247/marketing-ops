import type { Metadata } from "next";
import { WorkspaceShell } from "@/components/workspace-shell";

async function fetchWorkspace(id: string): Promise<{ name: string } | null> {
  try {
    const base = process.env.API_BASE_URL;
    const key = process.env.API_KEY;
    if (!base) return null;
    const headers: Record<string, string> = {};
    if (key) headers.Authorization = `Bearer ${key}`;
    const res = await fetch(`${base}/api/v1/workspaces/${encodeURIComponent(id)}`, {
      headers,
      cache: "no-store",
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { name?: unknown };
    return typeof data.name === "string" && data.name !== "" ? { name: data.name } : null;
  } catch {
    return null;
  }
}

export async function generateMetadata({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>): Promise<Metadata> {
  const { id } = await params;
  const workspace = await fetchWorkspace(id);
  return { title: workspace ? `${workspace.name} — Marketing Ops` : "Workspace — Marketing Ops" };
}

export default async function WorkspaceLayout({
  children,
  params,
}: Readonly<{ children: React.ReactNode; params: Promise<{ id: string }> }>) {
  const { id } = await params;
  const workspaceId = decodeURIComponent(id);
  const workspace = await fetchWorkspace(workspaceId);
  return (
    <WorkspaceShell workspaceId={workspaceId} workspaceName={workspace?.name ?? "Workspace"}>
      {children}
    </WorkspaceShell>
  );
}
