import type {
  DaemonStatus, AgentSummary, AgentView, AgentStatus,
  IterationSummary, IterationDetail, IterationLogs, UsageReport, AgentUsageReport, AgentEvent,
  StatusHistoryEvent,
  AgentLifecycleResult,
} from "./types";
import type { Daemon } from "./daemons";

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

// The "active daemon" the module-level helpers target. null = today's
// single-daemon same-origin, no-auth behavior (relative /api, no Authorization).
// Set by the host switcher; an empty registry leaves it null (backward-compatible).
let activeDaemon: Daemon | null = null;
export function setActiveDaemon(d: Daemon | null): void {
  activeDaemon = d;
}
export function getActiveDaemon(): Daemon | null {
  return activeDaemon;
}

// The origin of the LOCAL daemon. Empty ("") means same-origin, which is what a
// browser wants: relative /api paths through the dev proxy or the daemon's own
// listener. The desktop app has no same origin to fall back on — its page is
// served from tauri://localhost — so its Rust side hands the resolved
// http://127.0.0.1:PORT here at boot, before the first render.
let localBaseURL = "";
export function setLocalBaseURL(u: string): void {
  localBaseURL = u.replace(/\/+$/, "");
}
export function getLocalBaseURL(): string {
  return localBaseURL;
}

// ApiTarget selects which daemon a call addresses: undefined = the active
// daemon (module default, backward compatible), null = same-origin local,
// a Daemon = that host. Terminals-page calls pass explicit targets so they
// work cross-host without touching the active-daemon selection.
export type ApiTarget = Daemon | null | undefined;
export function resolveTarget(t: ApiTarget): Daemon | null {
  return t === undefined ? activeDaemon : t;
}

// resolveUrl prepends a daemon's baseURL (absolute cross-origin) or, for the
// local daemon, the configured local origin — which is "" in a browser, leaving
// the path relative exactly as before.
function resolveUrl(daemon: Daemon | null, path: string): string {
  if (!daemon) return localBaseURL + path;
  if (!daemon.baseURL) {
    throw new ApiError(0, "host_not_ready", `host ${daemon.label || daemon.id} is not ready`);
  }
  return daemon.baseURL.replace(/\/+$/, "") + path;
}
function authHeaders(daemon: Daemon | null): Record<string, string> {
  return daemon && daemon.token ? { Authorization: `Bearer ${daemon.token}` } : {};
}

// apiOn is the federation primitive: a fetch against an EXPLICIT daemon
// descriptor (null = same-origin no-auth). Unwraps the {ok,result}|{ok,error}
// envelope. Aggregate views fan this out per registered daemon.
export async function apiOn<T>(
  daemon: Daemon | null, method: string, path: string, body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = { ...authHeaders(daemon) };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res: Response;
  try {
    res = await fetch(resolveUrl(daemon, path), {
      method,
      cache: "no-store",
      headers: Object.keys(headers).length ? headers : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(0, "network_error", `network error: ${(e as Error).message}`);
  }
  const text = await res.text();
  let env: Envelope<T>;
  try {
    env = text ? (JSON.parse(text) as Envelope<T>) : { ok: res.ok };
  } catch {
    throw new ApiError(res.status, "bad_response", `non-JSON response (HTTP ${res.status})`);
  }
  if (!env.ok) {
    const err = env.error ?? { code: `http_${res.status}`, message: `HTTP ${res.status}` };
    throw new ApiError(res.status, err.code, err.message, err.details);
  }
  return env.result as T;
}

export async function apiRawOn(
  daemon: Daemon | null, method: string, path: string, body?: BodyInit,
): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(resolveUrl(daemon, path), {
      method, cache: "no-store", headers: authHeaders(daemon), body,
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
    if (env.error) { code = env.error.code; message = env.error.message; }
  } catch { /* keep stable HTTP fallback */ }
  throw new ApiError(response.status, code, message);
}

// Core fetch: targets the active daemon (null = relative same-origin /api, no
// auth — the M11 contract). Signature unchanged so every page keeps working.
export async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  return apiOn<T>(activeDaemon, method, path, body);
}

export const apiGet = <T>(path: string) => api<T>("GET", path);
export const apiPost = <T>(path: string, body?: unknown) => api<T>("POST", path, body);
export const apiPut = <T>(path: string, body?: unknown) => api<T>("PUT", path, body);
export const apiDelete = <T>(path: string, body?: unknown) => api<T>("DELETE", path, body);

export interface PluginOperatorArg {
  name: string;
  flag?: string;
  type: "string" | "integer" | "integer-list" | "boolean" | "secret-file";
  required?: boolean;
  help?: string;
}
export interface PluginOperatorCommand {
  path: string;
  summary: string;
  action: string;
  args?: PluginOperatorArg[];
}
export interface PluginSettingField {
  name: string;
  label: string;
  type: "string" | "password" | "integer-list";
  required?: boolean;
  help?: string;
}
export interface PluginSettingsContribution {
  title: string;
  status?: Array<{ name: string; label: string }>;
  sections?: Array<{
    title: string;
    fields?: PluginSettingField[];
    actions?: Array<{ label: string; action: string; fields?: string[] }>;
  }>;
}
export interface PluginContribution {
  name: string;
  description?: string;
  operator_commands?: PluginOperatorCommand[];
  settings?: PluginSettingsContribution;
}

export const getPluginContributionsOn = (target: ApiTarget) =>
  apiOn<{ plugins: PluginContribution[]; count: number }>(resolveTarget(target), "GET", "/api/plugin-contributions");
export const getPluginStatusOn = async (target: ApiTarget, name: string): Promise<Record<string, unknown>> => {
  const result = await apiOn<Record<string, unknown>>(
    resolveTarget(target), "GET", `/api/plugins/${encodeURIComponent(name)}/routes`,
  );
  const status = result.status;
  return status && typeof status === "object" && !Array.isArray(status)
    ? status as Record<string, unknown>
    : result;
};
export const runPluginActionOn = (target: ApiTarget, name: string, action: string, data: Record<string, unknown>) =>
  apiOn<Record<string, unknown>>(
    resolveTarget(target), "POST", `/api/plugins/${encodeURIComponent(name)}/action`,
    { name, action, data: JSON.stringify(data) },
  );

// Name-scoped path builder, preserved from v1: /api/agents/<enc(name)>[/<tail>].
export function agentApiPath(name: string, rest: string): string {
  const tail = rest.replace(/^\/+/, "").replace(/^api\//, "");
  const base = `/api/agents/${encodeURIComponent(name)}`;
  return tail ? `${base}/${tail}` : base;
}
export const agentGet = <T>(name: string, rest: string) => apiGet<T>(agentApiPath(name, rest));
export const agentPost = <T>(name: string, rest: string, body?: unknown) =>
  apiPost<T>(agentApiPath(name, rest), body);
export const agentPut = <T>(name: string, rest: string, body?: unknown) =>
  apiPut<T>(agentApiPath(name, rest), body);
export const agentDelete = <T>(name: string, rest: string, body?: unknown) =>
  apiDelete<T>(agentApiPath(name, rest), body);

// Name-scoped variants that address an EXPLICIT target instead of the active
// daemon (see ApiTarget) — used by cross-host views like /terminals.
export const agentPostOn = <T>(target: ApiTarget, name: string, rest: string, body?: unknown) =>
  apiOn<T>(resolveTarget(target), "POST", agentApiPath(name, rest), body);
export const agentGetOn = <T>(target: ApiTarget, name: string, rest: string) =>
  apiOn<T>(resolveTarget(target), "GET", agentApiPath(name, rest));
// force=true kills a live agent so it can be removed at all (a plain DELETE
// 400s on a running/idle agent: "stop it first or use --force"). purge=true
// hard-deletes the DB row + durable data (iterations/audit) + whole tree, so the
// agent actually disappears from the list — WITHOUT purge the daemon only does a
// preserving reset (strips the rebuildable tree, keeps the row listed as
// "stopped"). The /terminals Delete uses both: on this page an agent is meant to
// vanish, not linger as a stopped husk.
export const agentDeleteOn = <T>(target: ApiTarget, name: string, opts?: { force?: boolean; purge?: boolean }) => {
  const q = new URLSearchParams();
  if (opts?.force) q.set("force", "true");
  if (opts?.purge) q.set("purge", "true");
  const qs = q.toString();
  return apiOn<T>(resolveTarget(target), "DELETE", agentApiPath(name, "") + (qs ? `?${qs}` : ""));
};

// ---- Typed read helpers (core pages) ----
export const getDaemonStatus = () => apiGet<DaemonStatus>("/api/daemon/status");
export const listAgents = () => apiGet<{ agents: AgentSummary[]; count: number }>("/api/agents");

// ---- Agent creation (create forms; POST /api/agents = agent.run) ----
// The daemon keeps legacy scalar env/plugins forms for CLI and older callers,
// while the complete Desktop dialog uses structured values so commas, equals
// signs, whitespace, and newlines round-trip without reparsing.
export interface CreateAgentSpec {
  image: string;
  name?: string;
  cwd?: string;
  harness?: string;
  model?: string;
  effort?: string;
  interactive?: boolean;
  loop?: boolean;
  env?: string | Record<string, string>;
  plugins?: string | string[];
  timeout?: string;
  interval_s?: number;
  timeout_s?: number;
  hard_timeout_s?: number;
  on_timeout?: "restart" | "stop";
  on_error?: "restart" | "stop";
  max_idle_iterations?: number;
  user_prompt?: string;
  messages_batch?: number;
  messages_max_queue?: number;
  goal_enabled?: boolean;
  goal_wait_customer_timeout_s?: number;
  group?: string;
  alias?: string;
  notes?: string;
  color?: string;
}
export interface CreateAgentResult { name: string; state: string }
export const createAgent = (spec: CreateAgentSpec, target?: ApiTarget) =>
  apiOn<CreateAgentResult>(resolveTarget(target), "POST", "/api/agents", spec);
export const startAgent = (name: string, target?: ApiTarget) =>
  agentPostOn<AgentLifecycleResult>(target, name, "start");

// One image as listed by GET /api/images (the create form's image combobox).
export interface ImageRow {
  schema_version?: number;
  name: string;
  tag: string;
  digest?: string;
  built_at?: string;
  bare: boolean;
  source?: string;
  exportable?: boolean;
  source_digest?: string;
  source_cwd?: string;
  source_available?: boolean;
  current_agents?: string[];
  pending_agents?: string[];
}
export const listImagesOn = (target?: ApiTarget) =>
  apiOn<{ images: ImageRow[]; count: number }>(resolveTarget(target), "GET", "/api/images");
export const listImages = (target?: ApiTarget) => listImagesOn(target);

// One group as listed by GET /api/groups (the create form's optional group
// Select). Mirrors group.ls: name/lead plus a member count.
export interface GroupRow { name: string; lead: string; members: number }
export const listGroups = () => apiGet<{ groups: GroupRow[]; count: number }>("/api/groups");
export const getAgent = (name: string) => agentGet<AgentView>(name, "");
export const getAgentStatus = (name: string) => agentGet<AgentStatus>(name, "status");
export const setAgentCwdOn = (target: ApiTarget, name: string, value: string) =>
  agentPostOn<{ name: string; cwd: string }>(target, name, "cwd", { value });
export interface TimeoutExtension {
  id: string;
  timeout_deadline: string;
  hard_timeout_deadline: string;
  timeout_extensions: number;
  shim_sync: "success" | "pending";
}
export const extendIterationTimeout = (name: string, id: string) =>
  agentPost<TimeoutExtension>(name, `iterations/${encodeURIComponent(id)}/extend-timeout`);
export const listIterations = (name: string) =>
  agentGet<{ iterations: IterationSummary[]; count: number }>(name, "iterations");
export const getIteration = (name: string, id: string) =>
  agentGet<IterationDetail>(name, `iterations/${encodeURIComponent(id)}`);
export const getIterationLogs = (name: string, id: string) =>
  agentGet<IterationLogs>(name, `iterations/${encodeURIComponent(id)}/logs`);
export interface UsageParams { group?: string }
export const getUsage = (params: UsageParams = {}) => {
  const query = new URLSearchParams();
  if (params.group) query.set("group", params.group);
  const q = query.toString();
  return apiGet<UsageReport>(`/api/usage${q ? `?${q}` : ""}`);
};

// Per-agent Usage tab: grouped rows + time-bucketed series over one agent's
// AI usage. Drops empty params so the backend applies its defaults
// (group_by=iteration, bucket=1h, unbounded window).
export const getAgentUsage = (name: string, params: Record<string, string> = {}) => {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
  const s = q.toString();
  return agentGet<AgentUsageReport>(name, `usage${s ? `?${s}` : ""}`);
};

// ---- File upload into the agent cwd ----
export interface PushResult { path: string; abs: string; bytes: number }

// agentUploadFile base64-encodes a picked file and writes it under the agent's
// cwd at .tariboy/files/<name>, returning the absolute server-side path so
// the operator can paste it into the terminal.
export async function agentUploadFile(name: string, file: File, target?: ApiTarget): Promise<PushResult> {
  const buf = new Uint8Array(await file.arrayBuffer());
  let bin = "";
  for (const b of buf) bin += String.fromCharCode(b);
  const content = btoa(bin);
  return apiOn<PushResult>(resolveTarget(target), "PUT", agentApiPath(name, "files"), {
    path: `.tariboy/files/${file.name}`,
    content,
  });
}

// ---- Live config setters (persist now; launch-mode changes apply next start) ----
export const setAgentModel = (name: string, value: string) => agentPost(name, "model", { value });
export const setAgentEffort = (name: string, value: string) => agentPost(name, "effort", { value });
export const setAgentInteractive = (name: string, value: boolean) =>
  agentPost(name, "interactive", { value });
export const setAgentHarness = (name: string, value: string) =>
  agentPost(name, "harness", { value });

// ---- Alias / notes / status-history / loop-flag toggle (Overview page) ----
export const getAlias = (name: string) => agentGet<{ name: string; alias: string }>(name, "alias");
export const setAlias = (name: string, value: string) => agentPost(name, "alias", { value });
export const getNotes = (name: string) => agentGet<{ name: string; notes: string }>(name, "notes");
export const setNotes = (name: string, value: string) => agentPost(name, "notes", { value });
// Per-agent accent color. The setter's body field is `value` (matching the
// backend agent.color.set param); an empty string clears the color.
export const getColor = (name: string) => agentGet<{ name: string; color: string }>(name, "color");
export const setColor = (name: string, value: string) => agentPost(name, "color", { value });
export const getStatusHistory = (name: string) =>
  agentGet<{ events: StatusHistoryEvent[]; count: number }>(name, "status/history");
export const loopEnable = (name: string) => agentPost(name, "loop/enable");
export const loopDisable = (name: string) => agentPost(name, "loop/disable");

// ---- Durable agent scripts ----
export type ScriptMode = "once" | "every";
export type ScriptState = "active" | "completed" | "cancelled";
export type ScriptRunStatus = "pending" | "running" | "succeeded" | "failed" | "cancelled" | "timed_out" | "interrupted";
export interface ScriptRun {
  id: string;
  script_id: string;
  agent: string;
  status: ScriptRunStatus;
  cancel_requested?: boolean;
  pid?: number;
  exit_code?: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  log_path?: string;
}
export interface ScriptDefinition {
  id: string;
  agent: string;
  name: string;
  description: string;
  command: string;
  mode: ScriptMode;
  interval_seconds: number;
  quiet_exit?: number;
  state: ScriptState;
  created_at: string;
  next_run_at?: string;
  latest_run?: ScriptRun;
}
export interface RunOnceScriptSpec {
  script_name: string;
  description: string;
  command: string;
}
export interface ScheduleScriptSpec extends RunOnceScriptSpec { interval_seconds: number; quiet_exit?: number }
export const listAgentScripts = (name: string) =>
  agentGet<{ scripts: ScriptDefinition[]; count: number }>(name, "scripts");
export const runAgentScriptOnce = (name: string, spec: RunOnceScriptSpec) =>
  agentPost<{ script: ScriptDefinition; run: ScriptRun }>(name, "scripts/run", spec);
export const scheduleAgentScript = (name: string, spec: ScheduleScriptSpec) =>
  agentPost<{ script: ScriptDefinition; run: ScriptRun }>(name, "scripts/schedule", spec);
export const rerunAgentScript = (name: string, id: string) =>
  agentPost<ScriptRun>(name, `scripts/${encodeURIComponent(id)}/rerun`);
export const listAgentScriptRuns = (name: string, id: string) =>
  agentGet<{ runs: ScriptRun[]; count: number }>(name, `scripts/${encodeURIComponent(id)}/runs`);
export const getAgentScriptRun = (name: string, id: string) =>
  agentGet<ScriptRun>(name, `script-runs/${encodeURIComponent(id)}`);
export const getAgentScriptLog = (name: string, id: string) =>
  agentGet<{ run: ScriptRun; log: string }>(name, `script-runs/${encodeURIComponent(id)}/logs`);
export const downloadAgentScriptLog = async (name: string, id: string) =>
  (await apiRawOn(resolveTarget(undefined), "GET", agentApiPath(name, `script-runs/${encodeURIComponent(id)}/download`))).blob();
export const cancelAgentScript = (name: string, id: string) =>
  agentPost<{ id: string; cancelled: boolean }>(name, `script-targets/${encodeURIComponent(id)}/cancel`);
export const removeAgentScript = (name: string, id: string) =>
  agentDelete<{ id: string; removed: boolean }>(name, `scripts/${encodeURIComponent(id)}`);

// ---- Prompt layers (user-prompt / context / assembled preview) ----
export interface PromptLayer { name: string; sha256: string }
export type AgentPromptLayer = PromptLayer | ImageTemplateEntry;
export interface AgentPrompt { name: string; prompt: string; layers?: AgentPromptLayer[] }

export const agentPromptGet = (name: string) => agentGet<AgentPrompt>(name, "prompt");
export const agentUserPromptGet = (name: string) =>
  agentGet<{ name: string; user_prompt: string }>(name, "user-prompt");
export const agentUserPromptSet = (name: string, text: string) =>
  agentPost(name, "user-prompt", { text });
export const agentContextGet = (name: string) =>
  agentGet<{ name: string; context: string }>(name, "context");
export const agentContextSet = (name: string, text: string) =>
  agentPost(name, "context", { text });

// ---- File browser (CWD-jailed; singular /file namespace — /files is owned by
// cp push/pull). Backend: FB-1, commit 631fdeb. Reads shipped in FB-2; the
// write operations (save/create/rename/delete) are FB-3. Envelope error codes:
// bad_path / not_found / exists / is_dir / not_dir / bad_type. ----
export interface FileEntry { name: string; isDir: boolean; size: number; mtime: number }
export interface FileListing { path: string; entries: FileEntry[] }
export interface FileContent {
  path: string;
  kind: "text" | "binary" | "too_large";
  content: string;
  size: number;
}

// Root listing uses an empty path. The path rides as a query param; the daemon
// jails it under the agent's CWD.
export const agentFileList = (name: string, path: string) =>
  agentGet<FileListing>(name, `file/list?path=${encodeURIComponent(path)}`);
export const agentFileGet = (name: string, path: string) =>
  agentGet<FileContent>(name, `file?path=${encodeURIComponent(path)}`);

// Writes. Save/create/rename take their path(s) in the JSON body; delete takes
// the path in the query string (mirroring the read routes).
export const agentFileSave = (name: string, path: string, content: string) =>
  agentPut<{ path: string; saved: boolean }>(name, "file", { path, content });
export const agentFileCreate = (name: string, path: string, type: "file" | "dir") =>
  agentPost<{ path: string; created: boolean }>(name, "file", { path, type });
export const agentFileRename = (name: string, from: string, to: string) =>
  agentPost<{ from: string; to: string; renamed: boolean }>(name, "file/rename", { from, to });
export const agentFileDelete = (name: string, path: string) =>
  agentDelete<{ path: string; deleted: boolean }>(name, `file?path=${encodeURIComponent(path)}`);

// ---- Filesystem browser (cwd path autocomplete; $HOME-jailed, dirs only) ----
// Distinct from the agent-CWD file browser above: this lists directories under
// the daemon filesystem root (TARIBOY_FS_ROOT, default $HOME) so the create
// forms can pick an arbitrary cwd. Backend: internal/fsbrowser, GET
// /api/fs/list. Error codes: bad_path (403) / not_found (404) / not_dir (400).
export interface FsEntry { name: string; dir: boolean }
export interface FsListing { path: string; parent: string; entries: FsEntry[] }

// fsList lists the subdirectories of `path` under the filesystem root. An empty
// path lists the root itself; a relative path resolves from the root. The path
// rides as a query param and is omitted entirely when empty so the daemon
// applies its root default.
export const fsList = (path: string, target?: ApiTarget) =>
  apiOn<FsListing>(resolveTarget(target), "GET", `/api/fs/list${path ? `?path=${encodeURIComponent(path)}` : ""}`);

// ---- Images (read-only image detail: manifest / prompt / packed files) ----
// Backed by dev-t-qc4.1 routes: GET /api/images/{ref}, /{ref}/prompt,
// /{ref}/files, /{ref}/files/{path...}. `ref` is name:tag; it rides in the URL
// path so it is percent-encoded whole.
export interface ImageManifestPlugin { name: string; version?: string }
export interface ImageManifestSkill {
  name: string;
  description: string;
  source: string;
  category: string;
  archive_root: string;
  file_count: number;
  size: number;
  tree_sha256: string;
}
export interface ImageManifestHarness { type: string; model?: string; effort?: string; interactive: boolean }
export interface ImageManifestPolicy { tools_allow?: string[]; tools_deny?: string[] }
export interface ImageManifestEval { name: string; type: string; prompt: string }
export interface ImageManifest {
  schema_version: number;
  name: string;
  tag: string;
  digest?: string;
  built_at: string;
  parents: string[] | null;
  plugins: ImageManifestPlugin[] | null;
  skills: ImageManifestSkill[] | null;
  requires_secrets: string[] | null;
  harness?: ImageManifestHarness;
  env: Record<string, string> | null;
  policy?: ImageManifestPolicy;
  evals: ImageManifestEval[] | null;
  layers: PromptLayer[] | null;
  bare?: boolean;
  prompt_template_sha256?: string;
}
export interface ImageTemplateEntry { kind: "file" | "runtime"; runtime?: string; source?: string; category?: string; archive_path?: string; size?: number; sha256?: string }
export interface ImagePromptTemplate { schema_version: number; entries: ImageTemplateEntry[]; sha256: string }
export interface ImageProvenance { ref: string; digest?: string; source_cwd: string | null; built_at?: string; source_available: boolean }
export interface ImageBuildResult { name: string; tag: string; digest: string; layers: number }
export interface ImageDiagnostic { path: string; message: string }
export interface ImageValidationResult { valid:boolean; schema_version:number; plugins?:string[]; skills?:ImageManifestSkill[]; template?:ImagePromptTemplate|null; diagnostics?:ImageDiagnostic[]; warnings?:ImageDiagnostic[] }

export const validateImageDirectory = (input:{path:string;name:string;tag?:string}, target?: ApiTarget) =>
  apiOn<ImageValidationResult>(resolveTarget(target),"POST","/api/images/validate",input);
export const buildImageDirectory = (input:{path:string;name:string;tag?:string}, target?:ApiTarget) =>
  apiOn<ImageBuildResult>(resolveTarget(target),"POST","/api/images/build",input);
export const imageTemplateGet = (ref:string,target?:ApiTarget) =>
  apiOn<ImagePromptTemplate>(resolveTarget(target),"GET",`/api/images/${encodeURIComponent(ref)}/template`);
export const imageProvenanceGet = (ref:string,target?:ApiTarget) =>
  apiOn<ImageProvenance>(resolveTarget(target),"GET",`/api/images/${encodeURIComponent(ref)}/provenance`);
export interface AgentImageStatus { name:string; current:{ref:string;digest:string;error?:string}; pending:{ref:string;digest:string;error:string} }
export const agentImageStatusGetOn=(target:ApiTarget,name:string)=>agentGetOn<AgentImageStatus>(target,name,"image");
export const agentImageSetOn=(target:ApiTarget,name:string,image:string)=>agentPostOn<AgentImageStatus>(target,name,"image",{image});
export const agentImageCancelOn=(target:ApiTarget,name:string)=>apiOn<AgentImageStatus>(resolveTarget(target),"DELETE",agentApiPath(name,"image"));

// One member of an image's tar.gz. The backend returns a FLAT list of every
// member with its full slash-separated path; the image Files tab folds this
// into per-directory listings client-side (the FileBrowser expects listDir).
export interface ImageFileEntry { path: string; is_dir: boolean; size: number }

// Encode each path segment but keep the slashes: the read route is a
// {path...} wildcard, so nested paths must arrive slash-separated.
const encImagePath = (p: string) => p.split("/").map(encodeURIComponent).join("/");

export const imageManifestGetOn = (target: ApiTarget, ref: string) =>
  apiOn<ImageManifest>(
    resolveTarget(target), "GET", `/api/images/${encodeURIComponent(ref)}`,
  );
export const imageManifestGet = (ref: string, target?: ApiTarget) =>
  imageManifestGetOn(target, ref);

export const imagePromptGetOn = (target: ApiTarget, ref: string) =>
  apiOn<{ prompt: string }>(
    resolveTarget(target), "GET", `/api/images/${encodeURIComponent(ref)}/prompt`,
  );
export const imagePromptGet = (ref: string, target?: ApiTarget) =>
  imagePromptGetOn(target, ref);

export const imageFilesListOn = (target: ApiTarget, ref: string) =>
  apiOn<{ files: ImageFileEntry[] | null; count: number }>(
    resolveTarget(target), "GET", `/api/images/${encodeURIComponent(ref)}/files`,
  );
export const imageFilesList = (ref: string, target?: ApiTarget) =>
  imageFilesListOn(target, ref);

export const imageFileReadOn = (target: ApiTarget, ref: string, path: string) =>
  apiOn<{ path: string; content: string }>(
    resolveTarget(target),
    "GET",
    `/api/images/${encodeURIComponent(ref)}/files/${encImagePath(path)}`,
  );
export const imageFileRead = (ref: string, path: string, target?: ApiTarget) =>
  imageFileReadOn(target, ref, path);
export const removeImage = (ref: string, target?: ApiTarget) =>
  apiOn<{ removed: string }>(
    resolveTarget(target), "DELETE", `/api/images/${encodeURIComponent(ref)}`,
  );

// ---- Per-agent inbox (Messages tab; Phase P / P5 endpoints) ----
// One inbox row: the immutable message fields plus this agent's aggregated
// delivery state (§8.1). The backend omits optional fields when empty, so most
// are optional here; `result`/`processed_at` are present only on archive rows.
export interface InboxItem {
  id: string;
  channel: string;
  ts: string;
  source: string;
  type: string;
  text: string;
  attempts: number;
  dlq: boolean;
  produced_by_agent?: string;
  produced_in_iteration?: string;
  kind?: string; // event | request | reply
  subject?: Record<string, unknown>;
  data?: Record<string, unknown>;
  correlation_id?: string;
  in_reply_to?: string;
  reply_to?: string;
  deadline?: string;
  delivered_at?: string;
  processed_at?: string;
  result?: string;
}

// Backend status filter: pending → Queue, processed → Archive, dlq → DLQ.
export type InboxStatus = "pending" | "processed" | "dlq" | "all";

// List an agent's inbox for one sub-view. The backend returns rows newest-first.
export const agentInboxList = (name: string, status: InboxStatus, limit?: number, before?: string) => {
  const q = new URLSearchParams({ status });
  if (limit) q.set("limit", String(limit));
  if (before) q.set("before", before);
  return agentGet<{ messages: InboxItem[]; count: number }>(name, `inbox?${q.toString()}`);
};
// Mark a pending row processed (operator ack). `result` is mandatory; the
// backend prefixes it `operator:` so the audit trail distinguishes human acks.
export const agentInboxProcessed = (name: string, id: string, result: string) =>
  agentPost<{ id: string; processed_at: string; result: string }>(
    name, `inbox/${encodeURIComponent(id)}/processed`, { result });
// Reply to a row (operator). Publishes the reply and auto-processes the row.
export const agentInboxReply = (name: string, id: string, text: string) =>
  agentPost<{ id: string; channel: string; in_reply_to: string; correlation_id: string; replied: boolean }>(
    name, `inbox/${encodeURIComponent(id)}/reply`, { text });
// Requeue a DLQ'd row back to pending.
export const agentInboxRequeue = (name: string, id: string) =>
  agentPost<{ id: string; requeued: boolean }>(name, `inbox/${encodeURIComponent(id)}/requeue`, {});

// ---- SSE live events ----
// Opens an EventSource on <daemon.baseURL>/api/agents/<name>/events. For a
// cross-origin daemon the bearer rides in the URL as ?token= (EventSource cannot
// set an Authorization header — the daemon accepts it only on the /events route,
// and federated daemons therefore MUST run over TLS). Named events
// (iteration|message|stream|audit|proxy) invoke onEvent. The hub drops on a full
// buffer, so callers treat this as a refetch hint, not the source of truth.
export function subscribeAgentEventsOn(
  daemon: Daemon | null,
  name: string,
  types: string[],
  onEvent: (ev: AgentEvent) => void,
): () => void {
  if (daemon && !daemon.baseURL) return () => {};
  const params = new URLSearchParams();
  if (types.length) params.set("types", types.join(","));
  if (daemon && daemon.baseURL && daemon.token) params.set("token", daemon.token);
  const path = agentApiPath(name, "events");
  const base = daemon && daemon.baseURL ? daemon.baseURL.replace(/\/+$/, "") : localBaseURL;
  const qs = params.toString();
  const url = `${base}${path}${qs ? `?${qs}` : ""}`;
  const es = new EventSource(url);
  const kinds = ["message", "stream", "iteration", "audit", "proxy"];
  const handler = (e: MessageEvent) => {
    try {
      onEvent(JSON.parse(e.data) as AgentEvent);
    } catch {
      /* ignore malformed frame */
    }
  };
  for (const k of kinds) es.addEventListener(k, handler as EventListener);
  return () => es.close();
}

// Module-level subscriber: targets the active daemon (null = same-origin).
export function subscribeAgentEvents(
  name: string,
  types: string[],
  onEvent: (ev: AgentEvent) => void,
): () => void {
  return subscribeAgentEventsOn(activeDaemon, name, types, onEvent);
}
