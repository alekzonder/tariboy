import { describe, expect, it } from "vitest";
import { sshFooterAction, validateHttps, validateSsh } from "./serverFormValidation";
import { idleOperation, type HostOperation } from "./hostOperation";

describe("validateHttps", () => {
  it("needs a label", () => {
    expect(validateHttps("", "https://x", false)).toBe("label is required");
  });

  it("needs an http URL outside Android", () => {
    expect(validateHttps("prod", "", false)).toBe("base URL must start with http");
    expect(validateHttps("prod", "ftp://x", false)).toBe("base URL must start with http");
    expect(validateHttps("prod", "https://x", false)).toBeNull();
  });

  it("uses the Android URL rule inside the Android shell", () => {
    expect(validateHttps("prod", "http://example.com", false)).toBeNull();
    expect(validateHttps("prod", "http://example.com", true)).toMatch(/^Android allows plain http/);
    expect(validateHttps("prod", "http://127.0.0.1:9990", true)).toBeNull();
  });
});

describe("validateSsh", () => {
  it("needs a label, then an alias", () => {
    expect(validateSsh("", "")).toBe("label is required");
    expect(validateSsh("prod", "")).toBe("SSH alias is required");
    expect(validateSsh("prod", "prod-host")).toBeNull();
  });
});

describe("sshFooterAction", () => {
  const op = (patch: Partial<HostOperation>) => ({ ...idleOperation("provision"), ...patch });

  it("shows progress while an operation runs", () => {
    expect(sshFooterAction(op({ status: "running" }), true, true, true)).toBe("running");
  });

  it("offers the retry that matches a failed operation", () => {
    expect(sshFooterAction(op({ kind: "update", status: "failed" }), true, false, true)).toBe(
      "retry-update",
    );
    expect(sshFooterAction(op({ kind: "connect", status: "failed" }), true, false, true)).toBe(
      "retry-connect",
    );
  });

  it("adds a new host and reconnects an edited or disconnected one", () => {
    expect(sshFooterAction(op({ status: "failed" }), false, false, true)).toBe("add");
    expect(sshFooterAction(op({}), true, true, true)).toBe("reconnect");
    expect(sshFooterAction(op({}), true, false, false)).toBe("reconnect");
  });

  it("offers an update for an unchanged connected host", () => {
    expect(sshFooterAction(op({}), true, false, true)).toBe("update");
  });
});
