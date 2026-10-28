import type { Daemon } from "../daemons";
import { getLocalBaseURL } from "./hosts";

export class ApiError extends Error {
  status: number;
  code: string;
  details?: Record<string, unknown>;
  constructor(status: number, code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

interface Envelope<T> {
  ok: boolean;
  result?: T;
  error?: { code: string; message: string; details?: Record<string, unknown> };
}

// isConnectionError marks a failure to reach the host at all. The sidebar
// already reports an unreachable host, so background loads keep their last
// data quietly instead of toasting it every refresh.
export function isConnectionError(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.code === "host_not_ready" || error.code === "network_error")
  );
}

// resolveUrl prepends a daemon's baseURL (absolute cross-origin) or, for the
// local daemon, the configured local origin — which is "" in a browser, leaving
// the path relative.
function resolveUrl(daemon: Daemon | null, path: string): string {
  if (!daemon) return getLocalBaseURL() + path;
  if (!daemon.baseURL) {
    throw new ApiError(0, "host_not_ready", `host ${daemon.label || daemon.id} is not ready`);
  }
  return daemon.baseURL.replace(/\/+$/, "") + path;
}
function authHeaders(daemon: Daemon | null): Record<string, string> {
  return daemon && daemon.token ? { Authorization: `Bearer ${daemon.token}` } : {};
}

const READ_TIMEOUT_MS = 30_000;

// boundRead limits a GET to READ_TIMEOUT_MS and passes other methods through. A forwarded port can accept the
// connection and never answer; without a limit that read, and every refresh
// loop awaiting it, hangs until the app restarts. The caller's signal still
// cancels. Writes get no limit: they may legitimately run long.
function boundRead(method: string, signal?: AbortSignal) {
  if (method !== "GET") return { signal, timedOut: () => false, done: () => {} };
  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, READ_TIMEOUT_MS);
  const cancel = () => controller.abort();
  if (signal?.aborted) cancel();
  signal?.addEventListener("abort", cancel, { once: true });
  return {
    signal: controller.signal,
    timedOut: () => timedOut,
    done: () => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", cancel);
    },
  };
}

// request is the JSON transport against an EXPLICIT daemon descriptor (null =
// same-origin no-auth). Unwraps the {ok,result}|{ok,error} envelope.
async function request<T>(
  daemon: Daemon | null,
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = { ...authHeaders(daemon) };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const bound = boundRead(method, signal);
  let res: Response;
  let text: string;
  try {
    res = await fetch(resolveUrl(daemon, path), {
      method,
      cache: "no-store",
      headers: Object.keys(headers).length ? headers : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal: bound.signal,
    });
    text = await res.text();
  } catch (e) {
    if (e instanceof ApiError) throw e;
    if (bound.timedOut()) throw new ApiError(0, "network_error", "request timed out");
    throw new ApiError(0, "network_error", `network error: ${(e as Error).message}`);
  } finally {
    bound.done();
  }
  let env: Envelope<T>;
  try {
    env = text ? (JSON.parse(text) as Envelope<T>) : { ok: res.ok };
  } catch {
    throw new ApiError(res.status, "bad_response", `non-JSON response (HTTP ${res.status})`);
  }
  if (!env.ok) {
    const err = env.error ?? {
      code: `http_${res.status}`,
      message: `HTTP ${res.status}`,
    };
    throw new ApiError(res.status, err.code, err.message, err.details);
  }
  return env.result as T;
}

// requestRaw returns the successful Response itself, for archives, uploads and
// text exports; a failure still becomes an ApiError.
async function requestRaw(
  daemon: Daemon | null,
  method: string,
  path: string,
  body?: BodyInit,
): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(resolveUrl(daemon, path), {
      method,
      cache: "no-store",
      headers: authHeaders(daemon),
      body,
    });
  } catch (cause) {
    if (cause instanceof ApiError) throw cause;
    throw new ApiError(0, "network_error", `network error: ${(cause as Error).message}`);
  }
  if (response.ok) return response;
  let code = `http_${response.status}`;
  let message = `HTTP ${response.status}`;
  try {
    const env = JSON.parse(await response.text()) as Envelope<unknown>;
    if (env.error) {
      code = env.error.code;
      message = env.error.message;
    }
  } catch {
    /* keep stable HTTP fallback */
  }
  throw new ApiError(response.status, code, message);
}

// Name-scoped path builder: /api/agents/<enc(name)>[/<tail>].
export function agentApiPath(name: string, rest: string): string {
  const tail = rest.replace(/^\/+/, "").replace(/^api\//, "");
  const base = `/api/agents/${encodeURIComponent(name)}`;
  return tail ? `${base}/${tail}` : base;
}

// Transport is the request layer bound to one daemon. Domain modules receive it
// and never see a target of their own.
export interface Transport {
  readonly target: Daemon | null;
  request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T>;
  raw(method: string, path: string, body?: BodyInit): Promise<Response>;
  get<T>(path: string, signal?: AbortSignal): Promise<T>;
  post<T>(path: string, body?: unknown): Promise<T>;
  put<T>(path: string, body?: unknown): Promise<T>;
  delete<T>(path: string, body?: unknown): Promise<T>;
  agentGet<T>(name: string, rest: string, signal?: AbortSignal): Promise<T>;
  agentPost<T>(name: string, rest: string, body?: unknown): Promise<T>;
  agentPut<T>(name: string, rest: string, body?: unknown): Promise<T>;
  agentDelete<T>(name: string, rest: string, body?: unknown): Promise<T>;
}

export function bindTransport(target: Daemon | null): Transport {
  const call = <T>(method: string, path: string, body?: unknown, signal?: AbortSignal) =>
    signal
      ? request<T>(target, method, path, body, signal)
      : request<T>(target, method, path, body);
  return {
    target,
    request: call,
    raw: (method, path, body) => requestRaw(target, method, path, body),
    get: (path, signal) => call("GET", path, undefined, signal),
    post: (path, body) => call("POST", path, body),
    put: (path, body) => call("PUT", path, body),
    delete: (path, body) => call("DELETE", path, body),
    agentGet: (name, rest, signal) => call("GET", agentApiPath(name, rest), undefined, signal),
    agentPost: (name, rest, body) => call("POST", agentApiPath(name, rest), body),
    agentPut: (name, rest, body) => call("PUT", agentApiPath(name, rest), body),
    agentDelete: (name, rest, body) => call("DELETE", agentApiPath(name, rest), body),
  };
}

// readResult unwraps the envelope of a raw response (archive and upload routes).
export async function readResult<T>(response: Response): Promise<T> {
  let env: Envelope<T>;
  try {
    env = (await response.json()) as Envelope<T>;
  } catch {
    throw new ApiError(
      response.status,
      "bad_response",
      `non-JSON response (HTTP ${response.status})`,
    );
  }
  if (!env.ok || !env.result) {
    throw new ApiError(
      response.status,
      "bad_response",
      `invalid response (HTTP ${response.status})`,
    );
  }
  return env.result;
}
