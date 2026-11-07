import { WorkspaceDetailPanel } from "@/components/workspace-detail-panel";

export default async function WorkspaceSettingsPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return (
    <div className="grid gap-5">
      <div>
        <h2 className="text-lg font-bold tracking-tight text-slate-900">Workspace settings</h2>
        <p className="mt-1 text-sm text-slate-500">
          Workspace details, brand rules, and lifecycle. The lead engine, email, and integrations
          each have their own page.
        </p>
      </div>
      <WorkspaceDetailPanel workspaceId={decodeURIComponent(id)} />
    </div>
  );
}
