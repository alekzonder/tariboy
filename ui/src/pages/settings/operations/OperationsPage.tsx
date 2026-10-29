import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import type { OperationList } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/ui/status";
import { useOperationsSocket } from "@/hooks/useMailSocket";
import { errorText } from "@/lib/errors";
import {
  formatDuration,
  formatTime,
  kindLabel,
  operationTone,
  useCoalesced,
} from "./operationFormat";

const OPERATIONS_PAGE_SIZE = 50;
const CELL = "px-3 py-2";

/** Background operations of this server (image and workflow builds), newest first. */
export default function OperationsPage() {
  const api = useApi();
  const [page, setPage] = useState(1);
  const [list, setList] = useState<OperationList | null>(null);
  const [error, setError] = useState("");

  // The last loaded page, for the socket handler. load sets it with the
  // state, so a hint right after a load never sees the previous page.
  const listRef = useRef<{ list: OperationList | null; page: number }>({ list: null, page });
  const load = useCallback(async () => {
    try {
      const next = await api.operations.list(page, OPERATIONS_PAGE_SIZE);
      listRef.current = { list: next, page };
      setList(next);
      setError("");
    } catch (cause) {
      setError(errorText(cause));
    }
  }, [api, page]);
  const refresh = useCoalesced(load);
  useEffect(() => {
    void refresh();
  }, [refresh, load]);

  // Refetch on a hint while a listed operation runs, or when the first page
  // would gain an operation it does not list yet.
  const onHint = useCallback(
    (hint: { id: number }) => {
      const { list: current, page: shown } = listRef.current;
      const operations = current?.operations ?? [];
      const running = operations.some((operation) => operation.status === "running");
      const unseen = shown === 1 && !operations.some((operation) => operation.id === hint.id);
      if (running || unseen) void refresh();
    },
    [refresh],
  );
  const onOpen = useCallback(() => void refresh(), [refresh]);
  useOperationsSocket({ onHint, onOpen });

  const pages = list
    ? Math.max(1, Math.ceil(list.total / (list.limit || OPERATIONS_PAGE_SIZE)))
    : 1;
  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="space-y-1">
        <h2 className="text-xl font-semibold">Operations</h2>
        <p className="text-sm text-muted-foreground">
          Image and workflow builds run in the background on this server. Open one to read its log.
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {list === null ? (
        !error && <p className="text-sm text-muted-foreground">Loading operations…</p>
      ) : list.operations.length === 0 ? (
        <p className="text-sm text-muted-foreground">No operations</p>
      ) : (
        <OperationsTable list={list} />
      )}
      {list !== null && list.total > 0 && (
        <div className="flex items-center justify-end gap-2 text-sm">
          <span className="text-muted-foreground">
            Page {page} of {pages}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={page <= 1}
            onClick={() => setPage((value) => value - 1)}
          >
            Previous
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={page >= pages}
            onClick={() => setPage((value) => value + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}

function OperationsTable({ list }: { list: OperationList }) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            {["Kind", "Title", "Status", "Started", "Duration"].map((label) => (
              <th key={label} className={CELL}>
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {list.operations.map((operation) => (
            <tr key={operation.id} className="border-t">
              <td className={`${CELL} whitespace-nowrap`}>{kindLabel(operation.kind)}</td>
              <td className={CELL}>
                <Link className="text-primary hover:underline" to={String(operation.id)}>
                  {operation.title || `Operation ${operation.id}`}
                </Link>
              </td>
              <td className={CELL}>
                <StatusPill tone={operationTone(operation.status)}>{operation.status}</StatusPill>
              </td>
              <td className={`${CELL} text-xs whitespace-nowrap text-muted-foreground`}>
                {formatTime(operation.created_at)}
              </td>
              <td className={`${CELL} text-xs whitespace-nowrap`}>
                {formatDuration(operation.created_at, operation.finished_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
