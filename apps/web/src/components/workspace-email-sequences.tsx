"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  createEmailSequence,
  deleteEmailSequence,
  fetchEmailSequences,
  updateEmailSequence,
  type EmailSequence,
  type EmailSequenceStep,
  type SequenceSaveResult,
  type SequencesState,
} from "@/lib/email";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";
const PRIMARY_BUTTON =
  "rounded-lg bg-indigo-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 transition-colors";
const SECONDARY_BUTTON =
  "rounded-lg border border-slate-300 bg-white px-3.5 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 transition-colors";

// ─── Empty step factory ───────────────────────────────────────────────────────

function emptyStep(): EmailSequenceStep {
  return { delayDays: 1, subject: "", body: "" };
}

// ─── Step editor sub-component ────────────────────────────────────────────────

function StepEditor({
  index,
  step,
  total,
  onChange,
  onRemove,
  onMoveUp,
  onMoveDown,
}: {
  index: number;
  step: EmailSequenceStep;
  total: number;
  onChange: (step: EmailSequenceStep) => void;
  onRemove: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
}) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded-full bg-indigo-100 text-xs font-bold text-indigo-700">
            {index + 1}
          </span>
          <span className="text-xs font-semibold text-slate-600">
            {index === 0 ? "Initial email (sent immediately or on day 0)" : `Follow-up step`}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            disabled={index === 0}
            onClick={onMoveUp}
            title="Move up"
            className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30"
          >
            ↑
          </button>
          <button
            type="button"
            disabled={index === total - 1}
            onClick={onMoveDown}
            title="Move down"
            className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30"
          >
            ↓
          </button>
          <button
            type="button"
            onClick={onRemove}
            title="Remove step"
            className="rounded p-1 text-red-400 hover:bg-red-50 hover:text-red-700"
          >
            ✕
          </button>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-4">
        <div>
          <label className={labelClasses}>
            Delay (days)
          </label>
          <input
            type="number"
            min={0}
            max={365}
            value={step.delayDays}
            onChange={(e) => onChange({ ...step, delayDays: Math.max(0, parseInt(e.target.value, 10) || 0) })}
            className={`mt-1 ${inputClasses}`}
          />
          <p className="mt-0.5 text-[11px] text-slate-400">
            Days after {index === 0 ? "enrolment" : "previous step"}
          </p>
        </div>
        <div className="sm:col-span-3">
          <label className={labelClasses}>Subject line</label>
          <input
            type="text"
            value={step.subject}
            maxLength={200}
            placeholder="e.g. Quick question about your website"
            onChange={(e) => onChange({ ...step, subject: e.target.value })}
            className={`mt-1 ${inputClasses}`}
          />
        </div>
      </div>

      <div className="mt-3">
        <label className={labelClasses}>Email body</label>
        <textarea
          rows={4}
          value={step.body}
          maxLength={10000}
          placeholder={"Hi {{firstName}},\n\nI noticed…"}
          onChange={(e) => onChange({ ...step, body: e.target.value })}
          className={`mt-1 font-mono text-[13px] ${inputClasses}`}
        />
        <p className="mt-0.5 text-[11px] text-slate-400">
          Use <code className="rounded bg-slate-100 px-1">{"{{firstName}}"}</code>,{" "}
          <code className="rounded bg-slate-100 px-1">{"{{companyName}}"}</code> for merge tags.
        </p>
      </div>
    </div>
  );
}

// ─── Sequence editor form ─────────────────────────────────────────────────────

function SequenceForm({
  workspaceId,
  initial,
  onSave,
  onCancel,
}: {
  workspaceId: string;
  initial?: EmailSequence;
  onSave: (seq: EmailSequence) => void;
  onCancel: () => void;
}) {
  const apiBaseUrl = getApiBaseUrl();

  const [name, setName] = useState(initial?.name ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [steps, setSteps] = useState<EmailSequenceStep[]>(
    initial?.steps.length ? initial.steps : [emptyStep()],
  );
  const [saving, setSaving] = useState(false);
  const [problems, setProblems] = useState<string[]>([]);
  const [serverError, setServerError] = useState<string | null>(null);

  function updateStep(i: number, step: EmailSequenceStep) {
    setSteps((prev) => prev.map((s, idx) => (idx === i ? step : s)));
  }

  function removeStep(i: number) {
    setSteps((prev) => prev.filter((_, idx) => idx !== i));
  }

  function addStep() {
    setSteps((prev) => [...prev, emptyStep()]);
  }

  function moveStep(from: number, to: number) {
    setSteps((prev) => {
      const next = [...prev];
      const [item] = next.splice(from, 1);
      next.splice(to, 0, item);
      return next;
    });
  }

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setProblems([]);
    setServerError(null);

    const localProblems: string[] = [];
    if (!name.trim()) localProblems.push("Sequence name is required.");
    if (steps.length === 0) localProblems.push("Add at least one step.");
    if (localProblems.length) {
      setProblems(localProblems);
      return;
    }

    setSaving(true);
    let result: SequenceSaveResult;
    if (initial) {
      result = await updateEmailSequence(apiBaseUrl, initial.workspaceId, initial.id, {
        name: name.trim(),
        description: description.trim(),
        steps,
      });
    } else {
      result = await createEmailSequence(apiBaseUrl, workspaceId, {
        name: name.trim(),
        description: description.trim(),
        steps,
      });
    }
    setSaving(false);

    if (result.kind === "saved") {
      onSave(result.sequence);
    } else if (result.kind === "rejected") {
      setProblems(result.problems);
    } else {
      setServerError(result.message);
    }
  }

  return (
    <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label htmlFor="seq-name" className={labelClasses}>
            Sequence name
          </label>
          <input
            id="seq-name"
            type="text"
            value={name}
            maxLength={200}
            required
            placeholder="e.g. 3-step cold outreach"
            onChange={(e) => setName(e.target.value)}
            className={`mt-1 ${inputClasses}`}
          />
        </div>
        <div>
          <label htmlFor="seq-desc" className={labelClasses}>
            Description <span className="font-normal text-slate-400">(optional)</span>
          </label>
          <input
            id="seq-desc"
            type="text"
            value={description}
            maxLength={300}
            placeholder="Who this sequence is for"
            onChange={(e) => setDescription(e.target.value)}
            className={`mt-1 ${inputClasses}`}
          />
        </div>
      </div>

      <div className="space-y-3">
        {steps.map((step, i) => (
          <StepEditor
            key={i}
            index={i}
            step={step}
            total={steps.length}
            onChange={(s) => updateStep(i, s)}
            onRemove={() => removeStep(i)}
            onMoveUp={() => moveStep(i, i - 1)}
            onMoveDown={() => moveStep(i, i + 1)}
          />
        ))}
      </div>

      {steps.length < 20 && (
        <button
          type="button"
          onClick={addStep}
          className="w-full rounded-xl border-2 border-dashed border-slate-200 py-2.5 text-xs font-semibold text-slate-500 hover:border-indigo-300 hover:text-indigo-600 transition-colors"
        >
          + Add step
        </button>
      )}

      {problems.length > 0 && (
        <ul role="alert" className="list-disc space-y-1 pl-5 text-sm text-red-700">
          {problems.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
      )}
      {serverError && (
        <p role="alert" className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {serverError}
        </p>
      )}

      <div className="flex gap-2 pt-1">
        <button type="submit" disabled={saving} className={PRIMARY_BUTTON}>
          {saving ? "Saving…" : initial ? "Update sequence" : "Create sequence"}
        </button>
        <button type="button" onClick={onCancel} className={SECONDARY_BUTTON}>
          Cancel
        </button>
      </div>
    </form>
  );
}

// ─── Sequence card ────────────────────────────────────────────────────────────

function SequenceCard({
  seq,
  onEdit,
  onDelete,
}: {
  seq: EmailSequence;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
        <div className="min-w-0">
          <p className="truncate font-semibold text-slate-900">{seq.name}</p>
          {seq.description && (
            <p className="mt-0.5 text-xs text-slate-500">{seq.description}</p>
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className="rounded-full bg-indigo-50 px-2.5 py-0.5 text-[11px] font-semibold text-indigo-700">
            {seq.steps.length} step{seq.steps.length !== 1 ? "s" : ""}
          </span>
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            className="text-xs text-slate-500 hover:text-slate-800 underline"
          >
            {expanded ? "Hide" : "Preview"}
          </button>
          <button type="button" onClick={onEdit} className={SECONDARY_BUTTON}>
            Edit
          </button>
          <button
            type="button"
            onClick={onDelete}
            className="rounded-lg px-2.5 py-1.5 text-xs font-semibold text-red-700 hover:bg-red-50"
          >
            Delete
          </button>
        </div>
      </div>

      {expanded && (
        <div className="border-t border-slate-100 px-5 py-3">
          <ol className="space-y-3">
            {seq.steps.map((step, i) => (
              <li key={i} className="rounded-lg border border-slate-100 bg-slate-50 px-4 py-3">
                <div className="mb-1 flex items-center gap-2 text-xs">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-100 font-bold text-indigo-700">
                    {i + 1}
                  </span>
                  <span className="font-medium text-slate-700">
                    Day {step.delayDays}
                    {i > 0 ? " after previous step" : " (immediately)"}
                  </span>
                  {step.subject && (
                    <span className="ml-2 truncate text-slate-500">&ldquo;{step.subject}&rdquo;</span>
                  )}
                </div>
                {step.body && (
                  <pre className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap font-mono text-[11px] text-slate-600">
                    {step.body}
                  </pre>
                )}
              </li>
            ))}
          </ol>
        </div>
      )}
    </div>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export function WorkspaceEmailSequences({
  workspaceId,
}: Readonly<{ workspaceId: string }>) {
  const apiBaseUrl = getApiBaseUrl();
  const [state, setState] = useState<SequencesState>({ kind: "loading" });
  const [showForm, setShowForm] = useState(false);
  const [editingSeq, setEditingSeq] = useState<EmailSequence | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    setState(await fetchEmailSequences(apiBaseUrl, workspaceId));
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  function handleSaved(_seq: EmailSequence) {
    setShowForm(false);
    setEditingSeq(null);
    void load();
    document.getElementById("email-sequences-heading")?.scrollIntoView({ behavior: "smooth" });
  }

  async function handleDelete(id: string) {
    setDeleteError(null);
    setDeletingId(id);
    const result = await deleteEmailSequence(apiBaseUrl, workspaceId, id);
    setDeletingId(null);
    if (result.kind === "deleted") {
      void load();
    } else {
      setDeleteError(result.message);
    }
  }

  const sequences = state.kind === "loaded" ? state.sequences : [];

  return (
    <section
      id="email-sequences"
      aria-labelledby="email-sequences-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <div>
          <h2
            id="email-sequences-heading"
            className="text-xs font-semibold uppercase tracking-widest text-slate-500"
          >
            Email Drip Sequences
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            Multi-step email sequences for automated follow-ups. Assign sequences to leads when
            enrolling them.
          </p>
        </div>
        {!showForm && !editingSeq && (
          <button
            type="button"
            onClick={() => setShowForm(true)}
            className={SECONDARY_BUTTON}
          >
            + New sequence
          </button>
        )}
      </div>

      <div className="px-4 py-4 sm:px-5">
        {/* Create form */}
        {showForm && !editingSeq && (
          <div className="mb-5 rounded-xl border border-indigo-200 bg-indigo-50/40 p-5">
            <h3 className="mb-4 text-sm font-semibold text-slate-800">New drip sequence</h3>
            <SequenceForm
              workspaceId={workspaceId}
              initial={undefined}
              onSave={handleSaved}
              onCancel={() => setShowForm(false)}
            />
          </div>
        )}

        {/* Edit form */}
        {editingSeq && (
          <div className="mb-5 rounded-xl border border-indigo-200 bg-indigo-50/40 p-5">
            <h3 className="mb-4 text-sm font-semibold text-slate-800">
              Edit: {editingSeq.name}
            </h3>
            <SequenceForm
              workspaceId={workspaceId}
              initial={editingSeq}
              onSave={handleSaved}
              onCancel={() => setEditingSeq(null)}
            />
          </div>
        )}

        {/* States */}
        {state.kind === "loading" && (
          <div className="flex items-center gap-2 py-6 text-sm text-slate-500">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
            Loading sequences…
          </div>
        )}
        {state.kind === "unavailable" && (
          <p role="alert" className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
            {state.message}
          </p>
        )}

        {deleteError && (
          <p role="alert" className="mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
            {deleteError}
          </p>
        )}

        {state.kind === "loaded" && sequences.length === 0 && !showForm && (
          <div className="rounded-xl border border-dashed border-slate-200 py-10 text-center">
            <p className="text-sm font-medium text-slate-700">No sequences yet</p>
            <p className="mt-1 text-xs text-slate-500">
              Create your first drip sequence to automate lead follow-ups.
            </p>
            <button
              type="button"
              onClick={() => setShowForm(true)}
              className="mt-3 rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white hover:bg-indigo-700"
            >
              Create first sequence
            </button>
          </div>
        )}

        {sequences.length > 0 && !editingSeq && (
          <div className="space-y-3">
            {sequences.map((seq) => (
              <SequenceCard
                key={seq.id}
                seq={seq}
                onEdit={() => {
                  setShowForm(false);
                  setEditingSeq(seq);
                }}
                onDelete={() => {
                  if (deletingId) return;
                  void handleDelete(seq.id);
                }}
              />
            ))}
          </div>
        )}

        {deletingId && (
          <p className="mt-3 text-xs text-slate-400">Deleting…</p>
        )}
      </div>
    </section>
  );
}
