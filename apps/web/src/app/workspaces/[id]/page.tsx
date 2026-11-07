import { WorkspaceOverview } from "@/components/workspace-overview";

export default async function WorkspaceOverviewPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return <WorkspaceOverview workspaceId={decodeURIComponent(id)} />;
}
