import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DaemonProvider } from "@/components/DaemonProvider";
import DaemonsPage from "./DaemonsPage";
import { listDaemons, getDaemonToken } from "@/lib/daemons";
import * as desktop from "@/lib/desktop";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderPage() {
  return render(
    <DaemonProvider>
      <DaemonsPage />
    </DaemonProvider>,
  );
}

describe("DaemonsPage", () => {
  it("adds a daemon; token input is masked and never rendered back", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /add host/i }));
    fireEvent.change(screen.getByLabelText(/label/i), { target: { value: "prod" } });
    fireEvent.change(screen.getByLabelText(/base url/i), { target: { value: "https://prod:8765" } });
    const tokenInput = screen.getByLabelText(/token/i) as HTMLInputElement;
    expect(tokenInput.type).toBe("password");
    fireEvent.change(tokenInput, { target: { value: "sup3rsecret" } });
    await userEvent.click(screen.getByRole("button", { name: /^add$/i }));

    await waitFor(() => expect(screen.getByText("prod")).toBeInTheDocument());
    const daemons = await listDaemons();
    expect(daemons.map((d) => d.label)).toEqual(["prod"]);
    expect(await getDaemonToken(daemons[0].id)).toBe("sup3rsecret");
    // The token is NEVER rendered anywhere on the page.
    expect(screen.queryByText(/sup3rsecret/)).toBeNull();
  });

  it("removes a daemon", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /add host/i }));
    fireEvent.change(screen.getByLabelText(/label/i), { target: { value: "a" } });
    fireEvent.change(screen.getByLabelText(/base url/i), { target: { value: "https://a:1" } });
    fireEvent.change(screen.getByLabelText(/token/i), { target: { value: "ta" } });
    await userEvent.click(screen.getByRole("button", { name: /^add$/i }));
    await waitFor(() => expect(screen.getByText("a")).toBeInTheDocument());

    vi.spyOn(window, "confirm").mockReturnValue(true);
    await userEvent.click(screen.getByRole("button", { name: /remove/i }));
    await waitFor(async () => expect(await listDaemons()).toEqual([]));
  });

  it("shows native SSH connection failures instead of leaking a rejection", async () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    vi.spyOn(desktop, "hostsList").mockResolvedValue([{
      id: "ssh-1",
      label: "gpu",
      kind: "ssh",
      ssh_alias: "gpu-box",
      remote_install_dir: "~/.local/lib/tariboy",
      remote_port: 9990,
      https_base_url: "",
      last_daemon_version: "",
      state: "disconnected",
      base_url: "",
      local_port: 0,
      phase: "",
      platform: "",
      arch: "",
      prerequisites: [],
      message: "",
    }]);
    vi.spyOn(desktop, "hostConnect").mockRejectedValue(new Error("SSH timed out"));

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Connect gpu" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not connect to gpu: Error: SSH timed out",
    );
  });

  it("hides Update when the remote daemon already matches the Desktop bundle", async () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    vi.spyOn(desktop, "daemonState").mockResolvedValue({
      state: "ready",
      base_url: "http://127.0.0.1:9990",
      daemon_version: "0.11.5",
      app_version: "0.11.5",
      base_dir: "/tmp/tariboy",
      pid: 42,
      adopted: false,
      message: "",
    });
    vi.spyOn(desktop, "hostsList").mockResolvedValue([{
      id: "ssh-1",
      label: "gpu",
      kind: "ssh",
      ssh_alias: "gpu-box",
      remote_install_dir: "~/.local/lib/tariboy",
      remote_port: 9990,
      https_base_url: "",
      last_daemon_version: "0.11.5",
      state: "ready",
      base_url: "http://127.0.0.1:18444",
      local_port: 18444,
      phase: "connect",
      platform: "Linux",
      arch: "x86_64",
      prerequisites: [],
      message: "",
    }]);

    renderPage();

    expect(await screen.findByText("ready")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update gpu" })).toBeNull();
  });
});
