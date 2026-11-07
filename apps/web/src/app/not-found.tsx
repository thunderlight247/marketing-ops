import Link from "next/link";

// Next 15.5 can classify the generated /_not-found entry as dynamic, then fall
// back to a Pages Router /_document that does not exist in this app. Forcing
// the route static keeps the App Router 404 and avoids that fallback.
export const dynamic = "force-static";

export default function NotFound() {
  return (
    <div className="mx-auto max-w-lg py-16 text-center">
      <p className="text-xs font-semibold uppercase tracking-widest text-indigo-600">404</p>
      <h1 className="mt-2 text-2xl font-bold tracking-tight text-slate-900">Page not found</h1>
      <p className="mt-2 text-sm text-slate-600">
        That route does not exist. It may have moved, or the workspace link may be out of date.
      </p>
      <Link
        href="/"
        className="mt-6 inline-flex items-center justify-center rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
      >
        Back to Overview
      </Link>
    </div>
  );
}
