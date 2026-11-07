"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  listLeadEngineProgress,
  listWhatsAppSessions,
  type LeadEngineProgress,
  type WhatsAppSession,
} from "@/lib/leadEngine";
import { fetchWorkspaces, type Workspace } from "@/lib/workspaces";

/** A batch may wait up to the configured 300s between sends, so treat the
 * workspace as still running if the last attempt is inside this window. */
const SENDING_WINDOW_MS = 6 * 60 * 1000;

function isSending(progress: LeadEngineProgress | undefined): boolean {
  if (!progress) return false;
  if (progress.inFlight > 0) return true;
  if (progress.queued <= 0 || !progress.lastOutboundAt) return false;
  const last = new Date(progress.lastOutboundAt).getTime();
  return Number.isFinite(last) && Date.now() - last <= SENDING_WINDOW_MS;
}

function relativeTime(iso?: string): string | null {
  if (!iso) return null;
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return null;
  const minutes = Math.max(0, Math.round((Date.now() - then) / 60000));
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

/**
 * Workspace rail for running several workspaces at once.
 *
 * Every workspace keeps its own lead-engine batch and WhatsApp session, so a
 * link always opens a new tab: switching away can never replace or stop the
 * workspace already running here. Each card shows that workspace's number
 * (own or shared) and its live batch progress, so the one actually sending is
 * obvious without opening every tab.
 */
export function WorkspaceSwitcher({ currentId }: { currentId: string }) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  // null = not loaded yet (or the dispatcher is restarting).
  const [sessions, setSessions] = useState<WhatsAppSession[] | null>(null);
  const [progress, setProgress] = useState<LeadEngineProgress[] | null>(null);

  useEffect(() => {
    let active = true;
    const base = getApiBaseUrl();

    const loadSessions = async () => {
      try {
        const res = await listWhatsAppSessions(base);
        if (active) setSessions(res.sessions ?? []);
      } catch {
        if (active) setSessions(null);
      }
    };

    const loadProgress = async () => {
      try {
        const res = await listLeadEngineProgress(base);
        if (active) setProgress(res.progress ?? []);
      } catch {
        if (active) setProgress(null);
      }
    };

    void (async () => {
      const state = await fetchWorkspaces(base);
      if (active && state.kind === "loaded") {
        setWorkspaces(state.workspaces);
      }
      await loadSessions();
      await loadProgress();
    })();

    // Backlog changes while a batch runs; refresh quietly without touching
    // pairing state, which only changes on an explicit connect.
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") void loadProgress();
    }, 20000);

    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);

  if (workspaces.length < 2) return null;

  const connected = sessions?.filter((s) => s.connected).length ?? null;
  const sending =
    progress?.filter((p) => isSending(p)).length ?? 0;

  return (
    <section
      aria-label="Workspaces"
      className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-4 py-3">
        <div>
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-600">
            Run another workspace
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            Each tab runs on its own — same time, own WhatsApp session.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {sending > 0 && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-[11px] font-semibold text-emerald-800">
              <span
                className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-500 motion-reduce:animate-none"
                aria-hidden="true"
              />
              {sending} sending
            </span>
          )}
          {connected !== null && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-slate-100 px-2.5 py-1 text-[11px] font-semibold text-slate-700">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" aria-hidden="true" />
              {connected} connected
            </span>
          )}
        </div>
      </div>

      <ul className="grid gap-3 p-3 sm:grid-cols-2">
        {workspaces.map((workspace) => {
          const current = workspace.id === currentId;
          const session = sessions?.find((s) => s.workspaceId === workspace.id);
          const batch = progress?.find((p) => p.workspaceId === workspace.id);
          const activeNow = isSending(batch);
          const lastSend = relativeTime(batch?.lastOutboundAt);
          return (
            <li key={workspace.id}>
              <Link
                href={`/workspaces/${encodeURIComponent(workspace.id)}`}
                target="_blank"
                rel="noreferrer"
                aria-current={current ? "page" : undefined}
                title={
                  current
                    ? "You are here — open a copy in a new tab"
                    : "Open in a new tab so this workspace keeps running too"
                }
                className={`flex h-full flex-col justify-between gap-3 rounded-xl border p-3 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 ${
                  current
                    ? "border-indigo-300 bg-indigo-50/60"
                    : "border-slate-200 bg-white hover:border-indigo-200 hover:bg-slate-50"
                }`}
              >
                <div className="flex items-start justify-between gap-2">
                  <span className="line-clamp-2 text-sm font-semibold text-slate-900">
                    {workspace.name}
                  </span>
                  <span
                    className={`shrink-0 whitespace-nowrap text-[11px] font-semibold ${
                      current ? "text-indigo-700" : "text-slate-500"
                    }`}
                  >
                    {current ? "This tab" : "Open ↗"}
                  </span>
                </div>

                <div>
                  <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                    {session ? (
                      <>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${
                            session.source === "own"
                              ? "bg-emerald-100 text-emerald-800"
                              : "bg-indigo-100 text-indigo-700"
                          }`}
                        >
                          {session.source === "own" ? "Own number" : "Shared number"}
                        </span>
                        <span
                          className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                            session.connected ? "bg-emerald-500" : "bg-slate-300"
                          }`}
                          aria-hidden="true"
                        />
                        <span className="truncate text-[11px] text-slate-500">
                          {session.phone || "no number paired"}
                          {session.connected ? "" : " · offline"}
                        </span>
                      </>
                    ) : sessions !== null ? (
                      <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600">
                        No WhatsApp session
                      </span>
                    ) : null}
                  </div>

                  {batch && (batch.queued > 0 || batch.inFlight > 0) && (
                    <p className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px]">
                      <span
                        className={`h-1.5 w-1.5 rounded-full ${
                          activeNow
                            ? "animate-pulse bg-emerald-500 motion-reduce:animate-none"
                            : "bg-slate-300"
                        }`}
                        aria-hidden="true"
                      />
                      <span
                        className={
                          activeNow ? "font-semibold text-emerald-700" : "text-slate-600"
                        }
                      >
                        {batch.inFlight > 0
                          ? "Sending now"
                          : activeNow
                            ? "Batch running"
                            : "Queued"}
                      </span>
                      {batch.queued > 0 && (
                        <span className="text-slate-500">· {batch.queued} left</span>
                      )}
                      {lastSend && (
                        <span className="text-slate-400">· last send {lastSend}</span>
                      )}
                    </p>
                  )}
                </div>
              </Link>
            </li>
          );
        })}
      </ul>

      {sessions === null && (
        <p className="border-t border-slate-100 px-4 py-2 text-[11px] text-slate-500">
          Live pairing status is unavailable right now — workspaces can still be opened and run.
        </p>
      )}
    </section>
  );
}
