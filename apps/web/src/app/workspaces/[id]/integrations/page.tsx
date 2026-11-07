import { WorkspaceIntegrationsSection } from "@/components/workspace-integrations-section";

export default async function WorkspaceIntegrationsPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return <WorkspaceIntegrationsSection workspaceId={decodeURIComponent(id)} />;
}
