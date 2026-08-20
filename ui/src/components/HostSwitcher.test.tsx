import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DaemonProvider } from "./DaemonProvider";
import { HostSwitcher } from "./HostSwitcher";
import { addDaemon } from "@/lib/daemons";
import { getActiveDaemon } from "@/lib/api";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

describe("HostSwitcher", () => {
  it("lists the local daemon plus registered daemons and selecting one wires api.ts", async () => {
    await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "t0k" });
    render(
      <DaemonProvider>
        <HostSwitcher />
      </DaemonProvider>,
    );
    // Default active = local (same-origin) → api.ts active daemon is null.
    expect(getActiveDaemon()).toBeNull();

    await userEvent.click(screen.getByRole("combobox"));
    await userEvent.click(await screen.findByText("prod"));

    await waitFor(() => expect(getActiveDaemon()?.label).toBe("prod"));
    expect(getActiveDaemon()?.baseURL).toBe("https://prod:8765");
    expect(getActiveDaemon()?.token).toBe("t0k");
  });

  it("keeps the previous selection when the next host credentials cannot be resolved", async () => {
    const old = await addDaemon({ label: "old", baseURL: "https://old:8765", token: "old-token" });
    const broken = await addDaemon({ label: "broken", baseURL: "https://broken:8765", token: "bad-token" });
    localStorage.setItem("tariboy_active_daemon", old.id);

    render(
      <DaemonProvider>
        <HostSwitcher />
      </DaemonProvider>,
    );
    await waitFor(() => expect(getActiveDaemon()?.id).toBe(old.id));

    const getItem = Storage.prototype.getItem;
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (this: Storage, key: string) {
      if (this === sessionStorage && key.endsWith(broken.id)) throw new Error("Keychain unavailable");
      return getItem.call(this, key);
    });

    await userEvent.click(screen.getByRole("combobox"));
    await userEvent.click(await screen.findByText("broken"));

    await screen.findByRole("alert");
    expect(getActiveDaemon()?.id).toBe(old.id);
    expect(localStorage.getItem("tariboy_active_daemon")).toBe(old.id);
  });

  it("keeps the selector available while failing closed for a stale persisted remote id", () => {
    localStorage.setItem("tariboy_active_daemon", "removed-host");

    render(
      <DaemonProvider>
        <HostSwitcher />
      </DaemonProvider>,
    );

    expect(getActiveDaemon()).toMatchObject({ id: "removed-host", baseURL: "" });
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });
});
