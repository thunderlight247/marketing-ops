import { WorkspaceCalendar } from "@/components/workspace-calendar";

export default async function WorkspaceCalendarPage({
  params,
}: Readonly<{ params: Promise<{ id: string }> }>) {
  const { id } = await params;
  return <WorkspaceCalendar workspaceId={decodeURIComponent(id)} />;
}
