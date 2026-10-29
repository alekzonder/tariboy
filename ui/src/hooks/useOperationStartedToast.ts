import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import type { OperationStarted } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { hostToParam } from "@/lib/terminalsHost";
import { type OperationHint, useOperationsSocket } from "./useMailSocket";

const ENDED = new Set(["succeeded", "failed", "interrupted"]);

/** The page of one operation on a server, under that server's settings. */
function operationPath(hostParam: string, id: number): string {
  return `/servers/${encodeURIComponent(hostParam)}/settings/operations/${id}`;
}

/**
 * Report a build the daemon accepted and now runs in the background: a toast
 * with a link to the operation page, where its log and result appear. While
 * the caller stays mounted, it watches the operations socket and calls
 * `onSucceeded` when one of its builds ends well, so lists can reload.
 */
export function useOperationStartedToast(onSucceeded?: () => void) {
  const api = useApi();
  const navigate = useNavigate();
  const { hostId } = useParams();
  const host = hostId ?? hostToParam(api.target?.id ?? "");
  const pending = useRef(new Set<number>());
  const [watching, setWatching] = useState(false);
  const onSucceededRef = useRef(onSucceeded);
  useEffect(() => {
    onSucceededRef.current = onSucceeded;
  }, [onSucceeded]);

  const open = useCallback(
    (id: number) => ({ label: "Open operation", onClick: () => navigate(operationPath(host, id)) }),
    [host, navigate],
  );

  const onHint = useCallback(
    (hint: OperationHint) => {
      if (!pending.current.has(hint.id) || !ENDED.has(hint.status)) return;
      pending.current.delete(hint.id);
      if (pending.current.size === 0) setWatching(false);
      if (hint.status === "succeeded") onSucceededRef.current?.();
      else toast.error(`Build ${hint.status}`, { action: open(hint.id) });
    },
    [open],
  );
  // The socket replays nothing: a build that ended before it opened is read over HTTP.
  const onOpen = useCallback(() => {
    for (const id of pending.current) {
      void api.operations
        .get(id)
        .then((operation) => onHint({ id, status: operation.status, ts: "" }))
        .catch(() => undefined);
    }
  }, [api, onHint]);
  useOperationsSocket({ enabled: watching, onHint, onOpen });

  return useCallback(
    (started: OperationStarted) => {
      const { id, title } = started.operation;
      if (onSucceededRef.current) {
        pending.current.add(id);
        setWatching(true);
      }
      toast.success("Build started", { description: title, action: open(id) });
    },
    [open],
  );
}
