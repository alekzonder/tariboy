import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import type { OperationStarted } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { useMultiHost } from "@/lib/navRoutes";
import { hostToParam } from "@/lib/terminalsHost";
import { type OperationHint, useOperationsSocket } from "./useMailSocket";

const ENDED = new Set(["succeeded", "failed", "interrupted"]);

/** The route of an operation's page under Settings → Maintenance, naming
 *  the current host when the workspace has two or more. */
export function useOperationHref() {
  const api = useApi();
  const multiHost = useMultiHost();
  const host = multiHost ? `?host=${encodeURIComponent(hostToParam(api.target?.id ?? ""))}` : "";
  return useCallback((id: number) => `/settings/maintenance/operations/${id}${host}`, [host]);
}

/**
 * Report a build the daemon accepted and now runs in the background: a toast
 * with a link to the operation page, where its log and result appear. While
 * the caller stays mounted, it watches the operations socket and calls
 * `onSucceeded` when one of its builds ends well, so lists can reload, and
 * `onFailed` with the operation id when one ends otherwise.
 */
export function useOperationStartedToast(
  onSucceeded?: () => void,
  onFailed?: (id: number) => void,
) {
  const api = useApi();
  const navigate = useNavigate();
  const href = useOperationHref();
  const pending = useRef(new Set<number>());
  const [watching, setWatching] = useState(false);
  const onSucceededRef = useRef(onSucceeded);
  const onFailedRef = useRef(onFailed);
  useEffect(() => {
    onSucceededRef.current = onSucceeded;
    onFailedRef.current = onFailed;
  }, [onSucceeded, onFailed]);

  const open = useCallback(
    (id: number) => ({ label: "Open operation", onClick: () => navigate(href(id)) }),
    [href, navigate],
  );

  const onHint = useCallback(
    (hint: OperationHint) => {
      if (!pending.current.has(hint.id) || !ENDED.has(hint.status)) return;
      pending.current.delete(hint.id);
      if (pending.current.size === 0) setWatching(false);
      if (hint.status === "succeeded") {
        onSucceededRef.current?.();
        return;
      }
      toast.error(`Build ${hint.status}`, { action: open(hint.id) });
      onFailedRef.current?.(hint.id);
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
      if (onSucceededRef.current || onFailedRef.current) {
        pending.current.add(id);
        setWatching(true);
      }
      toast.success("Build started", { description: title, action: open(id) });
    },
    [open],
  );
}
