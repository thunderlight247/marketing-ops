import type { Metadata } from "next";
import { WorkspacesPanel } from "@/components/workspaces-panel";

export const metadata: Metadata = { title: "Workspaces" };

export default function WorkspacesPage() {
  return (
    <>
      <div className="mb-8">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Workspaces</h1>
        <p className="mt-1 text-sm text-slate-500">
          Every client gets their own workspace. Content, integrations, campaigns, and publishing
          history are isolated between workspaces.
        </p>
      </div>

      <WorkspacesPanel />
    </>
  );
}
