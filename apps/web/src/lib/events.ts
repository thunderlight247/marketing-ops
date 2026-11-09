import { useEffect, useRef } from "react";

export interface WorkspaceEvent {
  workspaceId: string;
  type:
    | "connected"
    | "lead_created"
    | "lead_updated"
    | "lead_discovered"
    | "pitch_dispatched"
    | "whatsapp_status"
    | "whatsapp_inbound"
    | "whatsapp_replied"
    | "lead_followup_overdue"
    | string;
  message?: string;
  timestamp: string;
}

/**
 * React hook that subscribes to real-time Server-Sent Events (SSE) for the active workspace.
 * Automatically cleans up on unmount or when the workspace changes.
 */
export function useWorkspaceEvents(
  workspaceId: string | null | undefined,
  onEvent: (event: WorkspaceEvent) => void,
) {
  const handlerRef = useRef(onEvent);
  handlerRef.current = onEvent;

  useEffect(() => {
    if (!workspaceId || typeof window === "undefined") {
      return;
    }

    const url = `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/events`;
    let es: EventSource | null = null;
    try {
      es = new EventSource(url);
    } catch {
      return;
    }

    es.onmessage = (event) => {
      try {
        const parsed: WorkspaceEvent = JSON.parse(event.data);
        handlerRef.current(parsed);
      } catch {
        // Ignore unparseable or keep-alive frames
      }
    };

    return () => {
      if (es) {
        es.close();
      }
    };
  }, [workspaceId]);
}
