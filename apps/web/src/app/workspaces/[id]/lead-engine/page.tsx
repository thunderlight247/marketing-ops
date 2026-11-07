import { WorkspaceLeadEngineSection } from "@/components/workspace-lead-engine-section";

export default async function WorkspaceLeadEnginePage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return <WorkspaceLeadEngineSection workspaceId={decodeURIComponent(id)} />;
}
