import { WorkspaceEmailSection } from "@/components/workspace-email-section";

export default async function WorkspaceEmailPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return <WorkspaceEmailSection workspaceId={decodeURIComponent(id)} />;
}
