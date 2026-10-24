import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MaintenanceSettingsCard } from "./MaintenanceSettings";
import { render } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import type { ApiClient, DaemonApi, MaintenanceSettings } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

const getMaintenance = vi.fn<DaemonApi["getMaintenance"]>();
const setMaintenance = vi.fn<DaemonApi["setMaintenance"]>();
const runMaintenance = vi.fn<DaemonApi["runMaintenance"]>();
const getIterationRetentionDefault = vi.fn<DaemonApi["getIterationRetentionDefault"]>();
const setIterationRetentionDefault = vi.fn<DaemonApi["setIterationRetentionDefault"]>();

// Each call goes through the client of the given host (local when null).
function renderCard(target: Daemon | null): ApiClient {
  const api = fakeApi({
    daemon: { getMaintenance, setMaintenance, runMaintenance, getIterationRetentionDefault, setIterationRetentionDefault },
  }, target);
  render(<MaintenanceSettingsCard />, { api });
  return api;
}

const defaults: MaintenanceSettings = {
  enabled: true, time: "03:00", keep_backups: 7, retention_days: 90, chat_retention_days: 7, compact: true,
  compact_threshold_pct: 10,
};
const lastRun = {
  started_at: "2026-09-24T03:00:00Z", finished_at: "2026-09-24T03:00:09Z", trigger: "scheduled",
  backup: "/b/tariboyd-20260924T030000Z.db", deleted: { tasks: 4, ai_requests: 120 },
  size_before: 157286400, size_after: 104857600, compacted: true,
};

beforeEach(() => {
  vi.clearAllMocks();
  getMaintenance.mockResolvedValue({ settings: defaults, last_run: lastRun });
  setMaintenance.mockImplementation(async (s) => s);
  getIterationRetentionDefault.mockResolvedValue({ keep_iterations: 50, keep_days: 0, max_bytes: 0 });
  setIterationRetentionDefault.mockImplementation(async (p) => p);
});

describe("MaintenanceSettingsCard", () => {
  it("shows the defaults and the last run", async () => {
    const api = renderCard(null);
    expect(await screen.findByLabelText("Nightly run time")).toHaveValue("03:00");
    expect(screen.getByLabelText("Keep data for (days)")).toHaveValue("90");
    expect(screen.getByLabelText("Backups to keep")).toHaveValue("7");
    expect(screen.getByLabelText("Keep chat history for (days)")).toHaveValue("7");
    expect(screen.getByRole("switch", { name: "Nightly backup and cleanup" })).toBeChecked();
    expect(screen.getByText(/tasks 4/)).toBeInTheDocument();
    expect(screen.getByText(/150\.0 MiB → 100\.0 MiB/)).toBeInTheDocument();
    expect(getMaintenance).toHaveBeenCalledWith();
    expect(api.target).toBeNull();
  });

  it("saves edited settings to the explicit target", async () => {
    const target = { id: "r", label: "R", baseURL: "https://r.test", token: "t" };
    const api = renderCard(target);
    fireEvent.change(await screen.findByLabelText("Keep data for (days)"), { target: { value: "120" } });
    fireEvent.change(screen.getByLabelText("Keep chat history for (days)"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("switch", { name: "Compact after cleanup" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(setMaintenance).toHaveBeenCalledWith(
      { ...defaults, retention_days: 120, chat_retention_days: 3, compact: false }));
    expect(api.target).toBe(target);
  });

  it("edits the server-wide iteration retention defaults", async () => {
    const target = { id: "r", label: "R", baseURL: "https://r.test", token: "t" };
    const api = renderCard(target);
    expect(await screen.findByLabelText("Keep iterations per agent")).toHaveValue("50");
    fireEvent.change(screen.getByLabelText("Keep iterations for (days)"), { target: { value: "30" } });
    expect(screen.getByRole("button", { name: "Run now" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(setIterationRetentionDefault).toHaveBeenCalledWith({ keep_iterations: 50, keep_days: 30, max_bytes: 0 }));
    expect(getIterationRetentionDefault).toHaveBeenCalledWith();
    expect(api.target).toBe(target);
  });

  it("shows a failed previous run on load", async () => {
    getMaintenance.mockResolvedValue({ settings: defaults, last_run: { ...lastRun, error: "backup: disk full" } });
    renderCard(null);
    expect(await screen.findByRole("alert")).toHaveTextContent("backup: disk full");
  });

  it("runs only saved settings", async () => {
    renderCard(null);
    fireEvent.change(await screen.findByLabelText("Backups to keep"), { target: { value: "3" } });
    expect(screen.getByRole("button", { name: "Run now" })).toBeDisabled();
  });

  it("runs now and shows a run error in place", async () => {
    runMaintenance.mockResolvedValue({ ...lastRun, trigger: "manual", error: "backup: disk full" });
    const api = renderCard(null);
    fireEvent.click(await screen.findByRole("button", { name: "Run now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("backup: disk full");
    expect(runMaintenance).toHaveBeenCalledWith();
    expect(api.target).toBeNull();
  });
});
