import { useContext, useEffect, useRef, useState } from "react";
import { ApiContext } from "@/components/apiContext";
import { getLocalBaseURL } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { subscribeResume } from "@/lib/resume";

const RECONNECT_MIN_MS = 250;
const RECONNECT_MAX_MS = 5000;

/** One letter written on this host: a frame of /api/mail/ws. Hand-written
 *  because the socket is outside the OpenAPI spec. It is a hint, not the
 *  letter: the client refetches over HTTP, which stays authoritative. */
export interface MailHint {
  id: string;
  from: string;
  to: string;
  kind: string;
  ts: string;
}

/** Something that can change an agent's state happened: an iteration started
 *  or ended, or a lifecycle event was recorded. The client refetches. */
export interface AgentStateHint {
  agent: string;
  type: string;
  ts: string;
}

/** A background operation started, wrote a log line, or ended: a frame of
 *  /api/operations/ws. Hand-written because the socket is outside the OpenAPI
 *  spec. It is a hint: the client refetches the operation over HTTP. */
export interface OperationHint {
  id: number;
  status: string;
  ts: string;
}

export type HintSocketStatus = "connecting" | "open" | "closed";

function hintWsUrl(target: Daemon | null, path: string): string {
  if (target && !target.baseURL) {
    throw new Error(`host ${target.label || target.id} is not ready`);
  }
  const toWs = (url: string) =>
    url
      .replace(/\/+$/, "")
      .replace(/^http:/, "ws:")
      .replace(/^https:/, "wss:");
  let base: string;
  if (target) {
    base = toWs(target.baseURL);
  } else {
    const local = getLocalBaseURL();
    base = local
      ? toWs(local)
      : `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}`;
  }
  const query = new URLSearchParams();
  if (target?.token) query.set("token", target.token);
  const qs = query.toString();
  return `${base}${path}${qs ? `?${qs}` : ""}`;
}

export function mailWsUrl(target: Daemon | null): string {
  return hintWsUrl(target, "/api/mail/ws");
}

type HintSocketOptions<T> = {
  /** An explicit host (null = same-origin local). Omitted, the socket watches
   *  the server of the surrounding ApiProvider. */
  target?: Daemon | null;
  enabled?: boolean;
  onHint: (hint: T) => void;
  onOpen?: () => void;
};

/**
 * Watch every letter on one host over a single socket. The socket replays
 * nothing, so the caller refetches on mount and on every reconnect; that, not a
 * cursor, is what makes a missed frame harmless.
 */
export function useMailSocket(options: HintSocketOptions<MailHint>): HintSocketStatus {
  return useHintSocket(
    "/api/mail/ws",
    (parsed) => {
      const hint = parsed as Partial<MailHint>;
      return hint && typeof hint.id === "string" ? (hint as MailHint) : null;
    },
    options,
  );
}

/**
 * Watch every agent's state on one host. While it is open, the agents list
 * and the agent status can poll slowly and refetch on a hint instead. A host
 * without this socket keeps it closed, and the caller keeps its short poll.
 */
export function useAgentStatesSocket(options: HintSocketOptions<AgentStateHint>): HintSocketStatus {
  return useHintSocket(
    "/api/agent-states/ws",
    (parsed) => {
      const hint = parsed as Partial<AgentStateHint>;
      return hint && typeof hint.agent === "string" ? (hint as AgentStateHint) : null;
    },
    options,
  );
}

/**
 * Watch every background operation (image and workflow builds) on one host.
 * The socket replays nothing, so the caller refetches on open as well.
 */
export function useOperationsSocket(options: HintSocketOptions<OperationHint>): HintSocketStatus {
  return useHintSocket(
    "/api/operations/ws",
    (parsed) => {
      const hint = parsed as Partial<OperationHint>;
      return hint && typeof hint.id === "number" ? (hint as OperationHint) : null;
    },
    options,
  );
}

function useHintSocket<T>(
  path: string,
  accept: (parsed: unknown) => T | null,
  { target, enabled = true, onHint, onOpen }: HintSocketOptions<T>,
): HintSocketStatus {
  const [status, setStatus] = useState<HintSocketStatus>(enabled ? "connecting" : "closed");
  const onHintRef = useRef(onHint);
  const onOpenRef = useRef(onOpen);
  const acceptRef = useRef(accept);
  useEffect(() => {
    onHintRef.current = onHint;
    onOpenRef.current = onOpen;
    acceptRef.current = accept;
  }, [onHint, onOpen, accept]);

  const provided = useContext(ApiContext);
  if (target === undefined && !provided)
    throw new Error("useApi must be used within an ApiProvider");
  // Keyed by the host's value: an equal descriptor (or an ordinary re-render)
  // keeps the socket, a new host, endpoint or token re-dials.
  const targetKey = JSON.stringify(target === undefined ? provided!.target : target);

  useEffect(() => {
    if (!enabled) {
      queueMicrotask(() => setStatus("closed"));
      return;
    }
    let alive = true;
    let socket: WebSocket | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let reconnectDelay = RECONNECT_MIN_MS;
    const socketTarget = JSON.parse(targetKey) as Daemon | null;

    const connect = () => {
      if (!alive) return;
      let url: string;
      try {
        url = hintWsUrl(socketTarget, path);
      } catch {
        setStatus("closed");
        return;
      }
      setStatus("connecting");
      const current = new WebSocket(url);
      socket = current;
      current.onopen = () => {
        if (!alive || socket !== current) return;
        reconnectDelay = RECONNECT_MIN_MS;
        setStatus("open");
        onOpenRef.current?.();
      };
      current.onmessage = (event) => {
        if (!alive || typeof event.data !== "string") return;
        let parsed: unknown;
        try {
          parsed = JSON.parse(event.data);
        } catch {
          return;
        }
        const hint = acceptRef.current(parsed);
        if (hint !== null) onHintRef.current(hint);
      };
      current.onclose = () => {
        if (!alive || socket !== current) return;
        socket = null;
        setStatus("closed");
        const wait = reconnectDelay;
        reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
        timer = setTimeout(connect, wait);
      };
    };

    connect();
    // Redial; the new socket's onOpen makes the caller refetch.
    const stopResume = subscribeResume(() => {
      if (timer !== undefined) clearTimeout(timer);
      reconnectDelay = RECONNECT_MIN_MS;
      const stale = socket;
      socket = null;
      stale?.close();
      connect();
    });
    return () => {
      stopResume();
      alive = false;
      if (timer !== undefined) clearTimeout(timer);
      socket?.close();
      socket = null;
    };
  }, [enabled, targetKey, path]);

  return status;
}
