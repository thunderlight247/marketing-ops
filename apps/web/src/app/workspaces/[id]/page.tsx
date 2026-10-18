import type { Metadata } from "next";
import { WorkspaceDetailPanel } from "@/components/workspace-detail-panel";

export const metadata: Metadata = { title: "Workspace" };

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
