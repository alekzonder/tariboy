import { useCallback, useEffect, useState } from "react";
import { Link, useLocation, useParams } from "react-router-dom";
import { toast } from "sonner";
import type { ApiClient, Operation } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/ui/status";
import { useOperationsSocket } from "@/hooks/useMailSocket";
import { errorText } from "@/lib/errors";
import { OperationDetails } from "./OperationDetails";
import { operationTone, useCoalesced } from "./operationFormat";
import { useOperationLog } from "./useOperationLog";

/** The route of one operation: a new id starts a new page, with an empty log. */
export default function OperationRoute() {
  const { id = "" } = useParams();
  const operationId = Number(id);
  if (!Number.isInteger(operationId) || operationId <= 0) {
    return (
      <p role="alert" className="text-sm text-destructive">
        Invalid operation id {id}.
      </p>
    );
  }
  return <OperationPage key={operationId} id={operationId} />;
}

const MAINTENANCE = "/settings/maintenance";

function OperationPage({ id }: { id: number }) {
  const api = useApi();
  const { search } = useLocation();
  const [operation, setOperation] = useState<Operation | null>(null);
  const [error, setError] = useState("");
  const log = useOperationLog(id);
  const { readMore } = log;

  const load = useCallback(async () => {
    try {
      setOperation(await api.operations.get(id));
      setError("");
    } catch (cause) {
      setError(errorText(cause));
      return;
    }
    await readMore();
  }, [api, id, readMore]);
  const refresh = useCoalesced(load);
  useEffect(() => {
    void refresh();
  }, [refresh]);

  const running = operation?.status === "running";
  const onHint = useCallback(
    (hint: { id: number }) => {
      if (hint.id === id) void refresh();
    },
    [id, refresh],
  );
  const onOpen = useCallback(() => void refresh(), [refresh]);
  useOperationsSocket({ enabled: running, onHint, onOpen });

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <Link to={{ pathname: MAINTENANCE, search }} className="text-sm text-primary hover:underline">
        ← Maintenance
      </Link>
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-xl font-semibold">{operation?.title || `Operation ${id}`}</h2>
        {operation && (
          <StatusPill tone={operationTone(operation.status)}>{operation.status}</StatusPill>
        )}
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {operation === null ? (
        !error && <p className="text-sm text-muted-foreground">Loading operation…</p>
      ) : (
        <>
          <OperationDetails operation={operation} />
          <OperationLogView id={id} {...log} />
        </>
      )}
    </div>
  );
}

async function downloadLog(api: ApiClient, id: number) {
  try {
    const blob = await api.operations.downloadLog(id);
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `operation-${id}.log`;
    anchor.click();
    URL.revokeObjectURL(url);
  } catch (cause) {
    toast.error(`Could not download log: ${errorText(cause)}`);
  }
}

function OperationLogView({
  id,
  content,
  available,
  error,
}: {
  id: number;
  content: string;
  available: boolean;
  error: string;
}) {
  const api = useApi();
  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h3 className="font-medium">Log</h3>
        <Button
          size="sm"
          variant="outline"
          disabled={!available}
          onClick={() => void downloadLog(api, id)}
        >
          Download log
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {!available ? (
        <p className="text-sm text-muted-foreground">Log unavailable</p>
      ) : (
        <pre
          aria-label="Operation log"
          className="max-h-[60vh] overflow-auto rounded border bg-muted/30 p-3 font-mono text-xs whitespace-pre-wrap"
        >
          {content || "(empty)"}
        </pre>
      )}
    </section>
  );
}
