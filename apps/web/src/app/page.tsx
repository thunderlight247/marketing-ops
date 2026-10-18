import Link from "next/link";
import { EmptyState } from "@/components/empty-state";
import { StatusPanel } from "@/components/status-panel";

export default function OverviewPage() {
  return (
    <>
      <h1 className="text-2xl font-semibold tracking-tight">Overview</h1>
      <p className="mt-1 max-w-prose text-slate-600">
        Plan, approve, publish, and measure marketing work for the clients you manage.
      </p>

      <div className="mt-8">
        <StatusPanel />
      </div>

      <div className="mt-6">
        <EmptyState
          title="No workspaces yet"
          body="This is the Milestone 0 foundation. No clients, integrations, or analytics are connected, and nothing on this page is sample data."
          note="Workspaces arrive in Milestone 2."
        />
      </div>

      <p className="mt-6 text-sm text-slate-600">
        Browse the planned sections from the menu, for example{" "}
        <Link
          href="/workspaces"
          className="font-medium text-teal-800 underline underline-offset-2 hover:text-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
        >
          Workspaces
        </Link>
        .
      </p>
    </>
  );
}
