import { WorkspaceEmailSection } from "@/components/workspace-email-section";
import { WorkspaceEmailSequences } from "@/components/workspace-email-sequences";

export default async function WorkspaceEmailPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  const workspaceId = decodeURIComponent(id);
  return (
    <div className="grid gap-6">
      <WorkspaceEmailSection workspaceId={workspaceId} />
      <WorkspaceEmailSequences workspaceId={workspaceId} />
    </div>
  );
}
