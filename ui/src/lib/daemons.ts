// Async host registry facade. The desktop app persists metadata in native
// hosts.json and HTTPS tokens in macOS Keychain; browser development keeps the
// historical localStorage/sessionStorage implementation as a fallback.

import {
  hostHasToken, hostRemove, hostSaveHttps, hostSessionCredentials, hostsList, isDesktop,
  type DesktopHostView,
} from "./desktop";

const META_KEY = "tariboy_daemons";
const ACTIVE_KEY = "tariboy_active_daemon";
const TOKEN_PREFIX = "tariboy_daemon_token_";
const REJECTED_CLEARTEXT_KEY = "tariboy_daemons_rejected_cleartext";

export interface DaemonMeta {
  id: string;
  label: string;
  baseURL: string;
  kind?: DesktopHostView["kind"];
  state?: DesktopHostView["state"];
  sshAlias?: string;
  phase?: string;
  platform?: string;
  arch?: string;
  prerequisites?: string[];
  message?: string;
  lastDaemonVersion?: string;
}

export interface Daemon extends DaemonMeta {
  token: string;
}

export function unresolvedDaemon(id: string, label = id): Daemon {
  return { id, label, baseURL: "", token: "" };
}

let migration: Promise<void> | null = null;
const sessionCache = new Map<string, Daemon>();

function loadBrowserMeta(): DaemonMeta[] {
  const raw = localStorage.getItem(META_KEY);
  if (!raw) return [];
  try {
    const value = JSON.parse(raw);
    return Array.isArray(value) ? (value as DaemonMeta[]) : [];
  } catch {
    return [];
  }
}

function saveBrowserMeta(list: DaemonMeta[]): void {
  localStorage.setItem(META_KEY, JSON.stringify(list));
}

function newBrowserId(): string {
  return `d_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

function normBaseURL(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

function viewMeta(view: DesktopHostView): DaemonMeta {
  return {
    id: view.id,
    label: view.label,
    baseURL: normBaseURL(view.base_url || view.https_base_url),
    kind: view.kind,
    state: view.state,
    sshAlias: view.ssh_alias,
    phase: view.phase,
    platform: view.platform,
    arch: view.arch,
    prerequisites: view.prerequisites,
    message: view.message,
    lastDaemonVersion: view.last_daemon_version,
  };
}

function requireNative<T>(value: T | null, command: string): T {
  if (value === null) throw new Error(`${command} is unavailable outside the desktop shell`);
  return value;
}

async function migrateLegacyDesktopRegistry(): Promise<void> {
  if (!isDesktop()) return;
  const legacy = loadBrowserMeta();
  const oldActive = localStorage.getItem(ACTIVE_KEY) ?? "";
  const native = requireNative(await hostsList(), "hosts_list");
  const activeMap = new Map<string, string>();
  const rejectedCleartext: DaemonMeta[] = [];

  for (const old of legacy) {
    const baseURL = normBaseURL(old.baseURL);
    // Native bearer hosts are HTTPS-only. Older Desktop builds could keep the
    // local daemon (or another cleartext endpoint) in Web Storage; trying to
    // import one now makes the entire registry hydration reject, hiding the
    // healthy implicit local host and every native SSH host. Quarantine only
    // its non-secret metadata and continue migrating safe records.
    if (!baseURL.startsWith("https://")) {
      rejectedCleartext.push({
        id: old.id,
        label: old.label,
        baseURL,
      });
      continue;
    }
    const token = sessionStorage.getItem(TOKEN_PREFIX + old.id) ?? "";
    const existing = native.find(
      (host) =>
        host.kind === "https" &&
        normBaseURL(host.https_base_url) === baseURL &&
        host.label === old.label.trim(),
    );
    const saved = requireNative(
      await hostSaveHttps(
        {
          id: existing?.id,
          label: old.label,
          https_base_url: baseURL,
        },
        token,
      ),
      "host_save_https",
    );
    activeMap.set(old.id, saved.id);
    sessionCache.set(saved.id, { ...viewMeta(saved), token });
  }

  // Delete legacy browser state only after every native metadata + Keychain
  // write above has succeeded. A partial failure leaves the source intact for
  // a safe retry; de-duplication prevents duplicate native records.
  if (legacy.length > 0) {
    if (rejectedCleartext.length > 0) {
      localStorage.setItem(REJECTED_CLEARTEXT_KEY, JSON.stringify(rejectedCleartext));
    }
    localStorage.removeItem(META_KEY);
    for (const old of legacy) sessionStorage.removeItem(TOKEN_PREFIX + old.id);
    const migratedActive = activeMap.get(oldActive) ?? "";
    if (migratedActive) localStorage.setItem(ACTIVE_KEY, migratedActive);
    else localStorage.removeItem(ACTIVE_KEY);
  }
}

async function ensureDesktopMigration(): Promise<void> {
  if (!isDesktop()) return;
  if (!migration) {
    migration = migrateLegacyDesktopRegistry().catch((error) => {
      migration = null;
      throw error;
    });
  }
  await migration;
}

export async function listDaemons(): Promise<DaemonMeta[]> {
  if (!isDesktop()) return loadBrowserMeta();
  await ensureDesktopMigration();
  return requireNative(await hostsList(), "hosts_list").map(viewMeta);
}

export async function addDaemon(input: {
  label: string;
  baseURL: string;
  token: string;
}): Promise<DaemonMeta> {
  const label = input.label.trim();
  const baseURL = normBaseURL(input.baseURL);
  if (!isDesktop()) {
    const meta = { id: newBrowserId(), label, baseURL };
    const list = loadBrowserMeta();
    list.push(meta);
    saveBrowserMeta(list);
    sessionStorage.setItem(TOKEN_PREFIX + meta.id, input.token);
    sessionCache.set(meta.id, { ...meta, token: input.token });
    return meta;
  }
  await ensureDesktopMigration();
  const saved = requireNative(
    await hostSaveHttps({ label, https_base_url: baseURL }, input.token),
    "host_save_https",
  );
  const meta = viewMeta(saved);
  sessionCache.set(meta.id, { ...meta, token: input.token });
  return meta;
}

export async function updateDaemon(
  id: string,
  input: { label: string; baseURL: string },
): Promise<void> {
  const label = input.label.trim();
  const baseURL = normBaseURL(input.baseURL);
  if (!isDesktop()) {
    const list = loadBrowserMeta();
    const index = list.findIndex((meta) => meta.id === id);
    if (index < 0) return;
    list[index] = { id, label, baseURL };
    saveBrowserMeta(list);
    const cached = sessionCache.get(id);
    if (cached) sessionCache.set(id, { id, label, baseURL, token: cached.token });
    return;
  }
  await ensureDesktopMigration();
  const saved = requireNative(
    await hostSaveHttps({ id, label, https_base_url: baseURL }),
    "host_save_https",
  );
  const cached = sessionCache.get(id);
  if (cached) sessionCache.set(id, { ...viewMeta(saved), token: cached.token });
}

export async function removeDaemon(id: string): Promise<void> {
  if (!isDesktop()) {
    saveBrowserMeta(loadBrowserMeta().filter((meta) => meta.id !== id));
    sessionStorage.removeItem(TOKEN_PREFIX + id);
  } else {
    await ensureDesktopMigration();
    await hostRemove(id);
  }
  sessionCache.delete(id);
  if ((await getActiveId()) === id) await setActiveId("");
}

export async function setDaemonToken(id: string, token: string): Promise<void> {
  if (!isDesktop()) {
    sessionStorage.setItem(TOKEN_PREFIX + id, token);
    const meta = loadBrowserMeta().find((item) => item.id === id);
    if (meta) sessionCache.set(id, { ...meta, token });
    return;
  }
  await ensureDesktopMigration();
  const meta = (await listDaemons()).find((item) => item.id === id);
  if (!meta) return;
  requireNative(
    await hostSaveHttps(
      { id, label: meta.label, https_base_url: meta.baseURL },
      token,
    ),
    "host_save_https",
  );
  sessionCache.set(id, { ...meta, token });
}

export async function getDaemonToken(id: string): Promise<string> {
  if (!isDesktop()) return sessionStorage.getItem(TOKEN_PREFIX + id) ?? "";
  await ensureDesktopMigration();
  const credentials = requireNative(
    await hostSessionCredentials(id),
    "host_session_credentials",
  );
  return credentials.token;
}

export async function clearDaemonToken(id: string): Promise<void> {
  if (!isDesktop()) {
    sessionStorage.removeItem(TOKEN_PREFIX + id);
    const meta = loadBrowserMeta().find((item) => item.id === id);
    if (meta) sessionCache.set(id, { ...meta, token: "" });
    return;
  }
  await setDaemonToken(id, "");
}

export async function hasDaemonToken(id: string): Promise<boolean> {
  if (!isDesktop()) return (sessionStorage.getItem(TOKEN_PREFIX + id) ?? "") !== "";
  await ensureDesktopMigration();
  return requireNative(await hostHasToken(id), "host_has_token");
}

export function peekActiveId(): string {
  return typeof localStorage === "undefined" ? "" : localStorage.getItem(ACTIVE_KEY) ?? "";
}

export async function getActiveId(): Promise<string> {
  if (!isDesktop()) return localStorage.getItem(ACTIVE_KEY) ?? "";
  await ensureDesktopMigration();
  return peekActiveId();
}

export async function setActiveId(id: string): Promise<void> {
  if (!isDesktop()) {
    if (id) localStorage.setItem(ACTIVE_KEY, id);
    else localStorage.removeItem(ACTIVE_KEY);
    return;
  }
  await ensureDesktopMigration();
  if (id) localStorage.setItem(ACTIVE_KEY, id);
  else localStorage.removeItem(ACTIVE_KEY);
}

export async function resolveDaemon(id: string): Promise<Daemon | null> {
  if (!id) return null;
  if (!isDesktop()) {
    const meta = loadBrowserMeta().find((item) => item.id === id);
    if (!meta) return null;
    const daemon = {
      ...meta,
      token: sessionStorage.getItem(TOKEN_PREFIX + id) ?? "",
    };
    sessionCache.set(id, daemon);
    return daemon;
  }
  await ensureDesktopMigration();
  const meta = (await listDaemons()).find((item) => item.id === id);
  if (!meta) return null;
  if (meta.kind === "ssh") {
    const daemon = { ...meta, token: "" };
    sessionCache.set(id, daemon);
    return daemon;
  }
  const credentials = requireNative(
    await hostSessionCredentials(id),
    "host_session_credentials",
  );
  const daemon = {
    ...meta,
    baseURL: normBaseURL(credentials.base_url),
    token: credentials.token,
  };
  sessionCache.set(id, daemon);
  return daemon;
}

export async function resolveActive(): Promise<Daemon | null> {
  const id = await getActiveId();
  if (!id) return null;
  return (await resolveDaemon(id)) ?? unresolvedDaemon(id);
}

/** Runtime-only lookup for render paths whose surrounding async fetch filled the cache. */
export function cachedDaemon(id: string): Daemon | null {
  return id ? sessionCache.get(id) ?? null : null;
}
