"use client";

import { useState } from "react";
import { WorkspaceContentSection } from "@/components/workspace-content-section";
import { WorkspaceContentCreatorSection } from "@/components/workspace-content-creator-section";
import { WorkspaceChannelCopySection } from "@/components/workspace-channel-copy-section";
import { WorkspaceCampaignsSection } from "@/components/workspace-campaigns-section";
import { WorkspaceLeadReplySection } from "@/components/workspace-lead-reply-section";
import { WorkspaceCampaignStrategistSection } from "@/components/workspace-campaign-strategist-section";
import { WorkspaceAgentsSection } from "@/components/workspace-agents-section";

export function WorkspaceContentHub({ workspaceId }: { workspaceId: string }) {
  const [contentRevision, setContentRevision] = useState(0);
  const [campaignRevision, setCampaignRevision] = useState(0);

  return (
    <div className="grid gap-5">
      <div>
        <h2 className="text-lg font-bold tracking-tight text-slate-900">Content & campaigns</h2>
        <p className="mt-1 text-sm text-slate-500">
          Draft, generate, review, and organise this workspace&apos;s content. Nothing publishes
          without an explicit approval.
        </p>
      </div>
      <div id="content">
        <WorkspaceContentSection key={`content-${contentRevision}`} workspaceId={workspaceId} />
      </div>
      <WorkspaceContentCreatorSection
        workspaceId={workspaceId}
        onCreated={() => setContentRevision((revision) => revision + 1)}
      />
      <WorkspaceChannelCopySection
        workspaceId={workspaceId}
        onFiled={() => setContentRevision((revision) => revision + 1)}
      />{" "}
      <div id="campaigns">
        <WorkspaceCampaignsSection
          key={`campaigns-${campaignRevision}`}
          workspaceId={workspaceId}
        />
      </div>
      <div id="lead-replies">
        <WorkspaceLeadReplySection
          workspaceId={workspaceId}
          onFiled={() => setCampaignRevision((revision) => revision + 1)}
        />
      </div>
      <WorkspaceCampaignStrategistSection
        workspaceId={workspaceId}
        onProposed={() => setCampaignRevision((revision) => revision + 1)}
      />
      <div id="agents">
        <WorkspaceAgentsSection />
      </div>
    </div>
  );
}
