export function EmptyState({
  title,
  body,
  icon,
  action,
}: Readonly<{
  title: string;
  body: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
}>) {
  return (
    <div className="flex flex-col items-center rounded-xl border border-dashed border-slate-200 bg-white px-6 py-14 text-center">
      {icon ? (
        <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-slate-100 text-slate-400">
          {icon}
        </div>
      ) : null}
      <h2 className="text-sm font-semibold text-slate-900">{title}</h2>
      <p className="mx-auto mt-1.5 max-w-sm text-sm leading-relaxed text-slate-500">{body}</p>
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  );
}
