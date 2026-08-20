import { beforeEach, describe, expect, it, vi } from "vitest";

const native = vi.hoisted(() => ({
  hosts: [] as Array<Record<string, unknown>>,
  tokens: new Map<string, string>(),
  saves: [] as Array<{ input: Record<string, unknown>; token?: string }>,
  credentialReads: [] as string[],
}));

vi.mock("./desktop", () => ({
  isDesktop: () => true,
  hostsList: async () => native.hosts,
  hostSaveHttps: async (input: Record<string, unknown>, token?: string) => {
    if (!String(input.https_base_url).startsWith("https://")) {
      throw new Error(
        "https_base_url must be a non-empty HTTPS URL beginning with https://",
      );
    }
    const id = (input.id as string | undefined) ?? `native-${native.hosts.length + 1}`;
    const view = {
      id,
      label: input.label,
      kind: "https",
      ssh_alias: "",
      remote_install_dir: "",
      remote_port: 0,
      https_base_url: input.https_base_url,
      last_daemon_version: "",
      state: "ready",
      base_url: input.https_base_url,
      local_port: 0,
      phase: "",
      platform: "",
      arch: "",
      prerequisites: [],
      message: "",
    };
    const index = native.hosts.findIndex((host) => host.id === id);
    if (index < 0) native.hosts.push(view);
    else native.hosts[index] = view;
    if (token !== undefined) native.tokens.set(id, token);
    native.saves.push({ input, token });
    return view;
  },
  hostSessionCredentials: async (id: string) => {
    native.credentialReads.push(id);
    const host = native.hosts.find((item) => item.id === id);
    return {
      base_url: host?.base_url ?? "",
      token: native.tokens.get(id) ?? "",
    };
  },
  hostHasToken: async (id: string) => native.tokens.has(id) && native.tokens.get(id) !== "",
  hostRemove: async (id: string) => {
    native.hosts = native.hosts.filter((host) => host.id !== id);
    native.tokens.delete(id);
    return null;
  },
}));

import {
  clearDaemonToken, getActiveId, hasDaemonToken, listDaemons, resolveDaemon,
} from "./daemons";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  native.hosts = [];
  native.tokens.clear();
  native.saves = [];
  native.credentialReads = [];
});

describe("desktop daemon registry migration", () => {
  it("imports legacy metadata/token, maps active id, then removes Web Storage", async () => {
    localStorage.setItem(
      "tariboy_daemons",
      JSON.stringify([
        { id: "legacy-1", label: "prod", baseURL: "https://prod:8765/" },
        { id: "legacy-http", label: "old local", baseURL: "http://127.0.0.1:9990/" },
      ]),
    );
    localStorage.setItem("tariboy_active_daemon", "legacy-1");
    sessionStorage.setItem("tariboy_daemon_token_legacy-1", "secret-token");
    sessionStorage.setItem("tariboy_daemon_token_legacy-http", "old-token");

    await expect(listDaemons()).resolves.toEqual([
      expect.objectContaining({
        id: "native-1",
        label: "prod",
        baseURL: "https://prod:8765",
        kind: "https",
        state: "ready",
      }),
    ]);
    expect(native.saves).toEqual([
      {
        input: { id: undefined, label: "prod", https_base_url: "https://prod:8765" },
        token: "secret-token",
      },
    ]);
    expect(await getActiveId()).toBe("native-1");
    expect(await resolveDaemon("native-1")).toMatchObject({
      id: "native-1",
      label: "prod",
      baseURL: "https://prod:8765",
      token: "secret-token",
    });
    expect(localStorage.getItem("tariboy_daemons")).toBeNull();
    expect(localStorage.getItem("tariboy_active_daemon")).toBe("native-1");
    expect(sessionStorage.getItem("tariboy_daemon_token_legacy-1")).toBeNull();
    expect(sessionStorage.getItem("tariboy_daemon_token_legacy-http")).toBeNull();
    expect(
      JSON.parse(localStorage.getItem("tariboy_daemons_rejected_cleartext") ?? "[]"),
    ).toEqual([
      {
        id: "legacy-http",
        label: "old local",
        baseURL: "http://127.0.0.1:9990",
      },
    ]);
  });

  it("checks the native credential instead of inferring it from host existence", async () => {
    native.hosts.push({
      id: "native-1",
      label: "prod",
      kind: "https",
      https_base_url: "https://prod",
      base_url: "https://prod",
    });
    native.tokens.set("native-1", "secret");

    await expect(hasDaemonToken("native-1")).resolves.toBe(true);
    await clearDaemonToken("native-1");
    await expect(hasDaemonToken("native-1")).resolves.toBe(false);
  });

  it("uses a ready SSH host tunnel URL without requesting direct credentials", async () => {
    native.hosts.push({
      id: "ssh-1",
      label: "gpu",
      kind: "ssh",
      ssh_alias: "gpu-box",
      https_base_url: "",
      state: "ready",
      base_url: "http://127.0.0.1:18444",
      phase: "connect",
      platform: "Linux",
      arch: "x86_64",
      prerequisites: [],
      message: "",
    });

    await expect(resolveDaemon("ssh-1")).resolves.toMatchObject({
      id: "ssh-1",
      label: "gpu",
      baseURL: "http://127.0.0.1:18444",
      token: "",
      kind: "ssh",
      state: "ready",
    });
    expect(native.credentialReads).toEqual([]);
  });

  it("keeps a disconnected SSH host identifiable and fail-closed", async () => {
    native.hosts.push({
      id: "ssh-down",
      label: "offline",
      kind: "ssh",
      ssh_alias: "offline-box",
      https_base_url: "",
      state: "disconnected",
      base_url: "",
      phase: "",
      platform: "",
      arch: "",
      prerequisites: [],
      message: "",
    });

    await expect(listDaemons()).resolves.toEqual([
      expect.objectContaining({
        id: "ssh-down",
        label: "offline",
        kind: "ssh",
        state: "disconnected",
        baseURL: "",
      }),
    ]);
    await expect(resolveDaemon("ssh-down")).resolves.toMatchObject({
      id: "ssh-down",
      label: "offline",
      baseURL: "",
      token: "",
    });
  });
});
