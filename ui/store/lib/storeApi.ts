// Store API client: relative /v1 fetch against the tariboy-store, bearer token
// held in sessionStorage (write-only in the UI — never rendered back, never in a
// URL, never logged), {ok,result}|{ok,error} envelope unwrap. Response shapes are
// authoritative from internal/storesvc/server.go + internal/image/manifest.go.

const TOKEN_KEY = "tariboy_store_token";

export class StoreApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "StoreApiError";
    this.status = status;
    this.code = code;
  }
}

export function setToken(t: string): void {
  sessionStorage.setItem(TOKEN_KEY, t);
}
export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY);
}
export function hasToken(): boolean {
  return !!sessionStorage.getItem(TOKEN_KEY);
}
function authHeader(): Record<string, string> {
  const t = sessionStorage.getItem(TOKEN_KEY);
  return t ? { Authorization: `Bearer ${t}` } : {};
}

interface Envelope<T> {
  ok: boolean;
  result?: T;
  error?: { code: string; message: string };
}

// PUBLIC_PATHS are never sent with the bearer token — /v1/info is reachable
// pre-auth (the login screen probes it to discover anon_pull) and must not
// leak the credential to a route that doesn't need it.
const PUBLIC_PATHS = new Set(["/v1/info"]);

export async function storeApi<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = PUBLIC_PATHS.has(path) ? {} : { ...authHeader() };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      cache: "no-store",
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    throw new StoreApiError(0, "network_error", `network error: ${(e as Error).message}`);
  }
  const text = await res.text();
  let env: Envelope<T>;
  try {
    env = text ? (JSON.parse(text) as Envelope<T>) : { ok: res.ok };
  } catch {
    throw new StoreApiError(res.status, "bad_response", `non-JSON response (HTTP ${res.status})`);
  }
  if (!env.ok) {
    const err = env.error ?? { code: `http_${res.status}`, message: `HTTP ${res.status}` };
    throw new StoreApiError(res.status, err.code, err.message);
  }
  return env.result as T;
}

// ---- Types (mirror the Go response shapes) ----
export interface StoreInfo {
  version: string;
  anon_pull: boolean;
}
export interface Repo {
  name: string;
  tags: string[];
}
export interface RepoCatalog {
  repos: Repo[];
  count: number;
}
export interface PushRow {
  tag: string;
  digest: string;
  built_at: string;
  pushed_at: string;
}
export interface TagList {
  name: string;
  tags: PushRow[];
}
export interface ManifestPlugin {
  name: string;
  version?: string;
}
export interface ManifestHarness {
  type: string;
  model?: string;
  effort?: string;
  interactive: boolean;
}
export interface ManifestEval {
  name: string;
  type: string;
  prompt: string;
}
export interface StoreManifest {
  schema_version: number;
  name: string;
  tag: string;
  digest?: string;
  built_at: string;
  parents: string[];
  plugins: ManifestPlugin[];
  requires_secrets: string[];
  harness: ManifestHarness;
  env: Record<string, string>;
  policy: { tools_allow?: string[]; tools_deny?: string[] };
  evals: ManifestEval[];
  layers: { name: string; sha256: string }[];
}

// ---- Typed helpers (real M13 routes + the M14 manifest/info additions) ----
export const getInfo = () => storeApi<StoreInfo>("GET", "/v1/info");
export const listRepos = () => storeApi<RepoCatalog>("GET", "/v1/images");
export const getTags = (name: string) =>
  storeApi<TagList>("GET", `/v1/images/${encodeURIComponent(name)}/tags`);
export const getManifest = (name: string, tag: string) =>
  storeApi<StoreManifest>(
    "GET",
    `/v1/images/${encodeURIComponent(name)}/${encodeURIComponent(tag)}/manifest`,
  );

// probeAuth reports whether the current token (or anon-pull) can read the catalog.
// A 401/403 means "not authorized" (show login); other errors propagate.
export async function probeAuth(): Promise<boolean> {
  try {
    await listRepos();
    return true;
  } catch (e) {
    if (e instanceof StoreApiError && (e.status === 401 || e.status === 403)) return false;
    throw e;
  }
}
