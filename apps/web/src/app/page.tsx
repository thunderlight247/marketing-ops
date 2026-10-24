import { StatusPanel } from "@/components/status-panel";
import { OverviewWorkspaces } from "@/components/overview-workspaces";
import { BatchPublishPanel } from "@/components/batch-publish-panel";

export default function OverviewPage() {
  return (
    <>
      <h1 className="text-2xl font-semibold tracking-tight">Marketing operations</h1>
      <p className="mt-1 max-w-prose text-slate-600">
        Manage multiple businesses independently. Add each business as a workspace, connect its own
        publishing destination, and review every post before it goes live.
      </p>

      <div className="mt-8">
        <StatusPanel />
      </div>

      <div className="mt-6">
        <OverviewWorkspaces />
      </div>

      <BatchPublishPanel />

      <section className="mt-6 rounded-lg border border-slate-200 bg-white p-5">
        <h2 className="text-base font-semibold text-slate-900">How publishing is set up</h2>
        <ol className="mt-3 grid gap-3 text-sm text-slate-700 sm:grid-cols-3">
          <li className="rounded-md bg-slate-50 p-3">
            <span className="font-semibold text-slate-900">1. Add a workspace</span>
            <p className="mt-1">
              Create a workspace for each business and add its website and brand details.
            </p>
          </li>
          <li className="rounded-md bg-slate-50 p-3">
            <span className="font-semibold text-slate-900">2. Connect its destination</span>
            <p className="mt-1">
              For Sanity, enter that project&apos;s Project ID, dataset, schema document type, and
              public article path prefix, then save its write token. Credentials are encrypted
              server-side and stay tied to that workspace.
            </p>
          </li>
          <li className="rounded-md bg-slate-50 p-3">
            <span className="font-semibold text-slate-900">3. Approve each post</span>
            <p className="mt-1">
              Create and review content, then approve the exact version before publishing. Select
              distinct approved posts to start their independent business-specific publishes
              together. Each result is tracked separately.
            </p>
          </li>
        </ol>
        <p className="mt-3 rounded-md border border-slate-200 bg-slate-50 p-3 text-xs leading-relaxed text-slate-600">
          You do not need to create or enter a Marketing Ops business ID; the app generates it when
          you create a workspace. Sanity Project ID and dataset identify that business&apos;s Sanity
          destination, schema document type, and public article path prefix. Its authorized API
          token is entered in the credentials form and encrypted server-side. The batch panel above
          can start multiple distinct approved posts concurrently, each through its selected
          business-specific destination. Each publish is independent; a failure for one post does
          not roll back successful publishes for others.
        </p>
        <p className="mt-3 text-xs text-slate-500">
          No real client business is preconfigured. Seeded workspaces are demo records for
          development; Nutrition Guide is only an example pilot noted in the project plan. Add your
          own businesses and authorized Sanity projects.
        </p>
      </section>
    </>
  );
}
