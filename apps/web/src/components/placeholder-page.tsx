import { findNavItem, type NavKey } from "@/lib/navigation";
import { EmptyState } from "./empty-state";

/** Shared body for sections that are navigation placeholders in Milestone 0. */
export function PlaceholderPage({ section }: Readonly<{ section: NavKey }>) {
  const item = findNavItem(section);
  return (
    <>
      <h1 className="text-2xl font-semibold tracking-tight">{item.label}</h1>
      <p className="mt-1 max-w-prose text-slate-600">{item.summary}</p>
      <div className="mt-8">
        <EmptyState
          title={item.emptyTitle}
          body={item.emptyBody}
          note={`Planned for ${item.plannedFor}.`}
        />
      </div>
    </>
  );
}
