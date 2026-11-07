import { WorkspaceAnalytics } from "@/components/workspace-analytics";
import { WorkspaceReportsSection } from "@/components/workspace-reports-section";
import { WorkspaceSeoAdvisorSection } from "@/components/workspace-seo-advisor-section";

export default async function WorkspaceAnalyticsPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  const workspaceId = decodeURIComponent(id);
  return (
    <div className="grid gap-5">
      <WorkspaceAnalytics workspaceId={workspaceId} />
      <div id="reports">
        <WorkspaceReportsSection workspaceId={workspaceId} />
      </div>
      <div id="seo">
        <WorkspaceSeoAdvisorSection workspaceId={workspaceId} />
      </div>
    </div>
  );
}
