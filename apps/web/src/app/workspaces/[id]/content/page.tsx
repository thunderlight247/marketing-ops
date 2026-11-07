import { WorkspaceContentHub } from "@/components/workspace-content-hub";

export default async function WorkspaceContentPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return <WorkspaceContentHub workspaceId={decodeURIComponent(id)} />;
}
