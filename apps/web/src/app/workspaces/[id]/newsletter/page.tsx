import { WorkspaceNewsletterSection } from "@/components/workspace-newsletter-section";

export default async function WorkspaceNewsletterPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  const workspaceId = decodeURIComponent(id);
  return (
    <div className="grid gap-6">
      <WorkspaceNewsletterSection workspaceId={workspaceId} />
    </div>
  );
}
