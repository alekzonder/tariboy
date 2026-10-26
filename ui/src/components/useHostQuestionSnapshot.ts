import { useCallback, useEffect, useRef, useState } from "react";
import { resolveDaemon, type Daemon } from "@/lib/daemons";
import { createApi, type ApiClient } from "@/lib/api";
import {
  CUSTOMER_QUESTION_RETRY_MS,
  readQuestionSnapshot,
  type FreshQuestion,
} from "./customerQuestionNotificationModel";

export type HostSnapshotListener = (
  hostId: string,
  label: string,
  attention: ReadonlyMap<string, number>,
  fresh: FreshQuestion[],
) => void;

/** Keeps one host's question inbox: a silent baseline at mount, retries until it
 *  lands, coalesced refreshes, and a silent re-baseline after a failed refresh. */
export function useHostQuestionSnapshot({
  hostId,
  label,
  onSnapshot,
  onRemove,
  registerRefresher,
}: {
  hostId: string;
  label: string;
  onSnapshot: HostSnapshotListener;
  onRemove: (hostId: string) => void;
  registerRefresher: (hostId: string, refresh: (() => Promise<void>) | null) => void;
}) {
  const [target, setTarget] = useState<Daemon | null | undefined>(undefined);
  const [health, setHealth] = useState<"healthy" | "recovering">("recovering");
  const [sequence, setSequence] = useState(0);
  const [socketEnabled, setSocketEnabled] = useState(false);
  const [retryRequest, setRetryRequest] = useState<{ generation: number; revision: number } | null>(
    null,
  );
  const observedIds = useRef<ReadonlySet<string>>(new Set<string>());
  const refreshState = useRef({ running: false, pending: false });
  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const generationRef = useRef(0);
  const labelRef = useRef(label);
  // The client of the last successful baseline; undefined while recovering.
  const refreshClientRef = useRef<ApiClient | undefined>(undefined);

  useEffect(() => {
    labelRef.current = label;
  }, [label]);

  const readAuthoritativeSnapshot = useCallback(
    async (client: ApiClient, baseline: boolean, generation: number) => {
      const [inbox, taskPage] = await Promise.all([
        client.tasks.listTaskNotifications(),
        client.tasks.listTasks({ limit: 1, status_view: "all" }),
      ]);
      if (generationRef.current !== generation) return false;
      if (baseline) refreshClientRef.current = client;
      const snapshot = readQuestionSnapshot(
        hostId,
        inbox.notifications ?? [],
        observedIds.current,
        baseline,
      );
      observedIds.current = snapshot.observed;
      onSnapshot(hostId, labelRef.current, snapshot.attention, snapshot.fresh);
      const nextSequence = taskPage.sequence ?? 0;
      if (baseline) setSequence(nextSequence);
      else setSequence((current) => Math.max(current, nextSequence));
      return true;
    },
    [hostId, onSnapshot],
  );

  const scheduleBaselineRetry = useCallback((generation: number) => {
    if (generationRef.current !== generation) return;
    if (retryTimer.current) clearTimeout(retryTimer.current);
    retryTimer.current = setTimeout(() => {
      setRetryRequest((current) => ({
        generation,
        revision: (current?.revision ?? 0) + 1,
      }));
    }, CUSTOMER_QUESTION_RETRY_MS);
  }, []);

  const refresh = useCallback(async () => {
    const refreshClient = refreshClientRef.current;
    if (refreshClient === undefined) return;
    if (refreshState.current.running) {
      refreshState.current.pending = true;
      return;
    }
    refreshState.current.running = true;
    const generation = generationRef.current;
    try {
      do {
        refreshState.current.pending = false;
        if (!(await readAuthoritativeSnapshot(refreshClient, false, generation))) break;
      } while (generationRef.current === generation && refreshState.current.pending);
    } catch {
      // Preserve the last successful snapshot, but recover through the same
      // silent baseline used at mount so an outage cannot replay alerts.
      if (generationRef.current === generation) {
        refreshClientRef.current = undefined;
        setHealth("recovering");
        setSocketEnabled(false);
        setTarget(undefined);
        scheduleBaselineRetry(generation);
      }
    } finally {
      refreshState.current.running = false;
    }
  }, [readAuthoritativeSnapshot, scheduleBaselineRetry]);

  useEffect(() => {
    registerRefresher(hostId, refresh);
    return () => registerRefresher(hostId, null);
  }, [hostId, refresh, registerRefresher]);

  const beginBaseline = useCallback(
    async (generation: number) => {
      try {
        const resolved = hostId ? await resolveDaemon(hostId) : null;
        if (hostId && !resolved) throw new Error(`host ${hostId} is unavailable`);
        if (generationRef.current !== generation) return;
        setTarget(resolved);
        const applied = await readAuthoritativeSnapshot(createApi(resolved), true, generation);
        if (generationRef.current === generation && applied) {
          setHealth("healthy");
          setSocketEnabled(true);
        }
      } catch {
        if (generationRef.current === generation) {
          refreshClientRef.current = undefined;
          setHealth("recovering");
          scheduleBaselineRetry(generation);
        }
      }
    },
    [hostId, readAuthoritativeSnapshot, scheduleBaselineRetry],
  );

  useEffect(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    void beginBaseline(generation);

    return () => {
      generationRef.current += 1;
      refreshClientRef.current = undefined;
      if (retryTimer.current) clearTimeout(retryTimer.current);
      retryTimer.current = null;
      onRemove(hostId);
    };
  }, [beginBaseline, hostId, onRemove]);

  useEffect(() => {
    if (retryRequest?.generation !== generationRef.current) return;
    void beginBaseline(retryRequest.generation);
  }, [beginBaseline, retryRequest]);

  const raiseSequence = useCallback((nextSequence: number) => {
    setSequence((current) => Math.max(current, nextSequence));
  }, []);

  return { target, health, sequence, socketEnabled, refresh, raiseSequence };
}
