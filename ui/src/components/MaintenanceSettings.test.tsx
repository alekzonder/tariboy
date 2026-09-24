import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MaintenanceSettingsCard } from "./MaintenanceSettings";
import { getMaintenanceOn, runMaintenanceOn, setMaintenanceOn, type MaintenanceSettings } from "@/lib/api";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  getMaintenanceOn: vi.fn(),
  setMaintenanceOn: vi.fn(),
  runMaintenanceOn: vi.fn(),
}));

const defaults: MaintenanceSettings = {
  enabled: true, time: "03:00", keep_backups: 7, retention_days: 90, compact: true, compact_threshold_pct: 10,
};
const lastRun = {
  started_at: "2026-09-24T03:00:00Z", finished_at: "2026-09-24T03:00:09Z", trigger: "scheduled",
  backup: "/b/tariboyd-20260924T030000Z.db", deleted: { tasks: 4, ai_requests: 120 },
  size_before: 157286400, size_after: 104857600, compacted: true,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getMaintenanceOn).mockResolvedValue({ settings: defaults, last_run: lastRun });
  vi.mocked(setMaintenanceOn).mockImplementation(async (_t, s) => s);
});

describe("MaintenanceSettingsCard", () => {
  it("shows the defaults and the last run", async () => {
    render(<MaintenanceSettingsCard target={null} />);
    expect(await screen.findByLabelText("Nightly run time")).toHaveValue("03:00");
    expect(screen.getByLabelText("Keep data for (days)")).toHaveValue("90");
    expect(screen.getByLabelText("Backups to keep")).toHaveValue("7");
    expect(screen.getByRole("switch", { name: "Nightly backup and cleanup" })).toBeChecked();
    expect(screen.getByText(/tasks 4/)).toBeInTheDocument();
    expect(screen.getByText(/150\.0 MiB → 100\.0 MiB/)).toBeInTheDocument();
    expect(getMaintenanceOn).toHaveBeenCalledWith(null);
  });

  it("saves edited settings to the explicit target", async () => {
    const target = { id: "r", label: "R", baseURL: "https://r.test", token: "t" };
    render(<MaintenanceSettingsCard target={target} />);
    fireEvent.change(await screen.findByLabelText("Keep data for (days)"), { target: { value: "120" } });
    fireEvent.click(screen.getByRole("switch", { name: "Compact after cleanup" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(setMaintenanceOn).toHaveBeenCalledWith(target, { ...defaults, retention_days: 120, compact: false }));
  });

  it("shows a failed previous run on load", async () => {
    vi.mocked(getMaintenanceOn).mockResolvedValue({ settings: defaults, last_run: { ...lastRun, error: "backup: disk full" } });
    render(<MaintenanceSettingsCard target={null} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("backup: disk full");
  });

  it("runs only saved settings", async () => {
    render(<MaintenanceSettingsCard target={null} />);
    fireEvent.change(await screen.findByLabelText("Backups to keep"), { target: { value: "3" } });
    expect(screen.getByRole("button", { name: "Run now" })).toBeDisabled();
  });

  it("runs now and shows a run error in place", async () => {
    vi.mocked(runMaintenanceOn).mockResolvedValue({ ...lastRun, trigger: "manual", error: "backup: disk full" });
    render(<MaintenanceSettingsCard target={null} />);
    fireEvent.click(await screen.findByRole("button", { name: "Run now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("backup: disk full");
    expect(runMaintenanceOn).toHaveBeenCalledWith(null);
  });
});
