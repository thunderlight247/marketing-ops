import type { Metadata } from "next";
import { DraftEditor } from "@/components/draft-editor";

export const metadata: Metadata = { title: "Content draft" };

export default async function ContentDraftPage({
  params,
}: Readonly<{ params: Promise<{ workspaceId: string; itemId: string }> }>) {
  const { workspaceId, itemId } = await params;
  const workspace = decodeURIComponent(workspaceId);
  const item = decodeURIComponent(itemId);
  return (
    <>
      <h1 className="text-2xl font-semibold tracking-tight">Draft</h1>
      <p className="mt-1 max-w-prose text-slate-600">
        Edit the brief for this content item. Saves are stored as numbered versions and the item
        stays a draft until you approve it for publishing.
      </p>
      <div className="mt-8">
        <DraftEditor workspaceId={workspace} itemId={item} />
      </div>
    </>
  );
}
