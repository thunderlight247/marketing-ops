import { StatusPanel } from "@/components/status-panel";
import { OverviewWorkspaces } from "@/components/overview-workspaces";
import { BatchPublishPanel } from "@/components/batch-publish-panel";

export default function OverviewPage() {
  return (
    <>
      <div className="mb-8">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Overview</h1>
        <p className="mt-1 text-sm text-slate-500">
          Platform status and your managed workspaces at a glance.
        </p>
      </div>

      <div className="space-y-5">
        <StatusPanel />

        <OverviewWorkspaces />

        <BatchPublishPanel />

        {/* How it works */}
        <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
          <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
              How publishing works
            </h2>
          </div>
          <ol className="grid gap-0 divide-y divide-slate-100 sm:grid-cols-3 sm:divide-y-0 sm:divide-x">
            {[
              {
                step: "1",
                title: "Add a workspace",
                body: "Create a workspace for each client. Each one keeps its brand, integrations, content, and publishing history completely separate.",
              },
              {
                step: "2",
                title: "Connect a destination",
                body: "Link the client's CMS, social accounts, or website. Credentials are encrypted server-side and scoped to that workspace only.",
              },
              {
                step: "3",
                title: "Approve and publish",
                body: "Draft content, review it, then approve the exact version. Nothing goes live without your explicit approval.",
              },
            ].map((item) => (
              <li key={item.step} className="flex gap-4 px-5 py-5">
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-xs font-bold text-indigo-700">
                  {item.step}
                </span>
                <div>
                  <p className="text-sm font-semibold text-slate-900">{item.title}</p>
                  <p className="mt-1 text-xs leading-relaxed text-slate-500">{item.body}</p>
                </div>
              </li>
            ))}
          </ol>
        </section>
      </div>
    </>
  );
}
