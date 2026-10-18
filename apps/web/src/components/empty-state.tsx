export function EmptyState({
  title,
  body,
  note,
}: Readonly<{ title: string; body: string; note?: string }>) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-10 text-center">
      <h2 className="text-base font-semibold text-slate-900">{title}</h2>
      <p className="mx-auto mt-2 max-w-md text-sm text-slate-600">{body}</p>
      {note ? <p className="mt-3 text-sm font-medium text-slate-700">{note}</p> : null}
    </div>
  );
}
