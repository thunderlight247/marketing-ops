import type { Metadata } from "next";
import { WorkspaceDetailPanel } from "@/components/workspace-detail-panel";

async function fetchWorkspaceName(id: string): Promise<string | null> {
  try {
    const base = process.env.API_BASE_URL;
    const key = process.env.API_KEY;
    if (!base) return null;
    const headers: Record<string, string> = {};
    if (key) headers["Authorization"] = `Bearer ${key}`;
    const res = await fetch(`${base}/api/v1/workspaces/${encodeURIComponent(id)}`, {
      headers,
      cache: "no-store",
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { name?: string };
    return data.name ?? null;
  } catch {
    return null;
  }
}

export async function generateMetadata({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>): Promise<Metadata> {
  const { id } = await params;
  const name = await fetchWorkspaceName(id);
  return { title: name ? `${name} — Marketing Ops` : "Workspace — Marketing Ops" };
}

export default async function WorkspaceDetailPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return (
    <>
      <h1 className="text-2xl font-semibold tracking-tight">Workspace</h1>
      <p className="mt-1 max-w-prose text-slate-600">
        {" "}
        This workspace groups the client&apos;s brand rules, integrations, and content. Everything
        shown here belongs only to this workspace.
      </p>
      <div className="mt-8">
        <WorkspaceDetailPanel workspaceId={decodeURIComponent(id)} />
      </div>
    </>
  );
}
