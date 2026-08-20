import { describe, it, expect, beforeEach } from "vitest";
import {
  addDaemon, listDaemons, removeDaemon, setDaemonToken, getDaemonToken,
  getActiveId, setActiveId, resolveDaemon, resolveActive,
} from "./daemons";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("daemon registry", () => {
  it("adds a daemon, persists metadata (not the token) in localStorage", async () => {
    const d = await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "t0k" });
    expect(d.id).toBeTruthy();
    expect((await listDaemons()).map((m) => m.label)).toEqual(["prod"]);
    // Token is NOT in the persisted metadata blob.
    const meta = localStorage.getItem("tariboy_daemons") ?? "";
    expect(meta).toContain("prod");
    expect(meta).not.toContain("t0k");
    // Token IS in sessionStorage under the per-id key.
    expect(await getDaemonToken(d.id)).toBe("t0k");
  });

  it("resolveDaemon merges metadata + token; empty id → null (same-origin)", async () => {
    const d = await addDaemon({ label: "a", baseURL: "https://a:1", token: "ta" });
    const r = await resolveDaemon(d.id);
    expect(r).toEqual({ id: d.id, label: "a", baseURL: "https://a:1", token: "ta" });
    expect(await resolveDaemon("")).toBeNull();
    expect(await resolveDaemon("nope")).toBeNull();
  });

  it("active id round-trips and resolveActive follows it; default is same-origin (null)", async () => {
    expect(await getActiveId()).toBe("");
    expect(await resolveActive()).toBeNull();
    const d = await addDaemon({ label: "a", baseURL: "https://a:1", token: "ta" });
    await setActiveId(d.id);
    expect(await getActiveId()).toBe(d.id);
    expect((await resolveActive())?.label).toBe("a");
  });

  it("resolves a stale non-local active id to a fail-closed placeholder", async () => {
    localStorage.setItem("tariboy_active_daemon", "removed-host");

    await expect(resolveActive()).resolves.toEqual({
      id: "removed-host",
      label: "removed-host",
      baseURL: "",
      token: "",
    });
  });

  it("removeDaemon drops metadata + token and clears active if it pointed there", async () => {
    const d = await addDaemon({ label: "a", baseURL: "https://a:1", token: "ta" });
    await setActiveId(d.id);
    await removeDaemon(d.id);
    expect(await listDaemons()).toEqual([]);
    expect(await getDaemonToken(d.id)).toBe("");
    expect(await getActiveId()).toBe("");
  });

  it("setDaemonToken updates the write-only token without touching metadata", async () => {
    const d = await addDaemon({ label: "a", baseURL: "https://a:1", token: "ta" });
    await setDaemonToken(d.id, "tb");
    expect(await getDaemonToken(d.id)).toBe("tb");
    expect((await resolveDaemon(d.id))?.token).toBe("tb");
    expect((await listDaemons())[0].label).toBe("a");
  });
});
