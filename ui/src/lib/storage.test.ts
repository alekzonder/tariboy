import { afterEach, describe, expect, it } from "vitest";
import {
  readKey,
  readKeyResult,
  removeKey,
  storageKeyFamilies,
  storageKeys,
  writeKey,
  type StorageKey,
} from "./storage";

const keys = Object.values(storageKeys) as StorageKey<unknown>[];
const families = Object.values(storageKeyFamilies);

function breakStorage(name: "localStorage" | "sessionStorage", error = new Error("blocked")) {
  const original = Object.getOwnPropertyDescriptor(globalThis, name)!;
  Object.defineProperty(globalThis, name, {
    configurable: true,
    get() {
      throw error;
    },
  });
  return () => Object.defineProperty(globalThis, name, original);
}

function failWrites(name: "localStorage" | "sessionStorage") {
  // jsdom stores any property set on a Storage as an item, so swap the object.
  const original = Object.getOwnPropertyDescriptor(globalThis, name)!;
  Object.defineProperty(globalThis, name, {
    configurable: true,
    value: {
      getItem: () => null,
      removeItem: () => {},
      setItem: () => {
        throw new DOMException("full", "QuotaExceededError");
      },
    },
  });
  return () => Object.defineProperty(globalThis, name, original);
}

let restore: (() => void) | undefined;
afterEach(() => {
  restore?.();
  restore = undefined;
  localStorage.clear();
  sessionStorage.clear();
});

describe("storage key registry", () => {
  it("names every key once", () => {
    const names = keys.map((key) => key.name);
    expect(new Set(names).size).toBe(names.length);
  });

  it("ends a versioned name with its version", () => {
    for (const key of keys) {
      if (key.version !== null) expect(key.name).toMatch(new RegExp(`:v${key.version}$`));
    }
  });

  it("keeps family prefixes apart from every other key", () => {
    for (const family of families) {
      for (const key of keys) expect(key.name.startsWith(family.prefix)).toBe(false);
      for (const other of families) {
        if (other !== family) expect(other.prefix.startsWith(family.prefix)).toBe(false);
      }
    }
  });

  it("builds family keys under the family prefix", () => {
    expect(storageKeyFamilies.tuiDraft.key("worker", "manager").name).toBe(
      "tui:draft:worker:manager",
    );
    expect(storageKeyFamilies.daemonToken.key("d_1").name).toBe("tariboy_daemon_token_d_1");
    expect(storageKeyFamilies.daemonToken.key("d_1").area).toBe("session");
  });

  it("reads back what it wrote for every key", () => {
    for (const key of keys) {
      if (key.fallback === null) continue;
      expect(writeKey(key, key.fallback)).toBe(true);
      expect(readKey(key)).toEqual(key.fallback);
    }
    expect(writeKey(storageKeys.terminalsServerOrder, { version: 1, ids: ["a"] })).toBe(true);
    expect(localStorage.getItem("terminals:server-order:v1")).toBe('{"version":1,"ids":["a"]}');
    expect(readKey(storageKeys.terminalsServerOrder)).toEqual({ version: 1, ids: ["a"] });
  });

  it("keeps each key in its own area", () => {
    writeKey(storageKeys.storeToken, "secret");
    expect(sessionStorage.getItem("tariboy_store_token")).toBe("secret");
    expect(localStorage.getItem("tariboy_store_token")).toBeNull();
  });
});

describe("storage access", () => {
  it("returns the fallback for a missing value", () => {
    expect(readKey(storageKeys.uiMode)).toBe("simple");
    expect(readKey(storageKeys.workspaces)).toBeNull();
  });

  it("returns the fallback for broken JSON", () => {
    localStorage.setItem("app:workspaces:v1", "{not json");
    expect(readKey(storageKeys.workspaces)).toBeNull();
  });

  it("returns the fallback for a value the parser rejects", () => {
    localStorage.setItem("tariboy-theme", "purple");
    expect(readKey(storageKeys.theme)).toBe("system");
    localStorage.setItem("tariboy:team-import", '{"id":1}');
    expect(readKey(storageKeys.teamImport)).toBeNull();
  });

  it("returns the fallback and reports failure when the storage throws", () => {
    restore = breakStorage("localStorage");
    expect(readKey(storageKeys.theme)).toBe("system");
    expect(readKeyResult(storageKeys.desktopAutoDownload)).toEqual({ value: true, ok: false });
    expect(writeKey(storageKeys.theme, "dark")).toBe(false);
    expect(() => removeKey(storageKeys.theme)).not.toThrow();
  });

  it("reports a full storage as a failed write", () => {
    restore = failWrites("sessionStorage");
    expect(writeKey(storageKeys.storeToken, "secret")).toBe(false);
  });

  it("removes a value", () => {
    writeKey(storageKeys.uiMode, "expert");
    removeKey(storageKeys.uiMode);
    expect(localStorage.getItem("app:ui-mode:v1")).toBeNull();
  });
});
