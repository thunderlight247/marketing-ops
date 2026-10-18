import type { Metadata } from "next";
import { WorkspacesPanel } from "@/components/workspaces-panel";

export const metadata: Metadata = { title: "Workspaces" };

export default function WorkspacesPage() {
  return (
    <>
      <h1 className="text-2xl font-semibold tracking-tight">Workspaces</h1>
      <p className="mt-1 max-w-prose text-slate-600">
        Every client business gets its own workspace. Integrations, content, campaigns, and reports
        live inside it — nothing is shared between workspaces.
      </p>

      <div className="mt-8">
        <WorkspacesPanel />
      </div>
    </>
  );
}
