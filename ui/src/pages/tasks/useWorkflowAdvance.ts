import { useState } from "react";
import { useApi } from "@/components/apiContext";
import type { WorkflowView } from "@/lib/api";
import { useFollowRequest, type Notice } from "./workflowShared";

/**
 * The customer's advance with its message, followed until its checks finish.
 * A request that settled without applying leaves a notice. The outcome block
 * of the workflow panel and the Needs you card share it, so both send the
 * same request with `from` set to the status the customer saw.
 */
export function useWorkflowAdvance(
  taskKey: string,
  view: Pick<WorkflowView, "status">,
  onChanged: () => void,
  onRefresh: () => void,
) {
  const api = useApi();
  const [message, setMessage] = useState("");
  const { checking, follow } = useFollowRequest(taskKey);
  // A notice belongs to the status it was given in; once the view shows
  // another status it no longer applies and is not shown.
  const [held, setHeld] = useState<{ status: string; notice: Notice } | null>(null);
  const notice = held?.status === view.status ? held.notice : null;
  const setNotice = (next: Notice | null, status = view.status) =>
    setHeld(next && { status, notice: next });

  const advance = (outcome: string) => {
    setNotice(null);
    void follow(
      () => api.workflows.advanceTask(taskKey, outcome, message, view.status),
      (result) => {
        switch (result.kind) {
          case "applied":
            setMessage("");
            onChanged();
            return;
          case "result":
            setNotice({ kind: "result", request: result.request });
            onRefresh();
            return;
          case "timeout":
            setNotice({ kind: "timeout", requestId: result.requestId });
            return;
          case "error":
            // status_changed names the status the task is in now, where the
            // refetched view lands, so its message survives that refetch.
            setNotice(
              { kind: "error", label: result.label, message: result.message },
              result.status ?? view.status,
            );
            if (result.stale) onRefresh();
        }
      },
    );
  };
  return { message, setMessage, checking, notice, advance };
}
