import type { Metadata } from "next";
import { WorkspacesPanel } from "@/components/workspaces-panel";

export const metadata: Metadata = { title: "Workspaces" };

export default function WorkspacesPage() {
  return (
    <>
      <div className="mb-8">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Workspaces</h1>
        <p className="mt-1 text-sm text-slate-500">
          Your connected client workspaces. Open one to work inside it, or add another workspace
          below.
        </p>
      </div>

      <WorkspacesPanel />
    </>
  );
}
