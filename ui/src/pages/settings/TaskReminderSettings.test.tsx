import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TaskReminderSettings from "./TaskReminderSettings";
import { getTaskReminderPolicyOn, setTaskReminderPolicyOn } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  getTaskReminderPolicyOn: vi.fn(),
  setTaskReminderPolicyOn: vi.fn(),
}));

const remote: Daemon = {
  id: "remote-1",
  label: "Production",
  baseURL: "https://production.example",
  token: "secret",
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTaskReminderPolicyOn).mockResolvedValue({ enabled: false, idle_threshold_s: 300 });
  vi.mocked(setTaskReminderPolicyOn).mockResolvedValue({ enabled: false, idle_threshold_s: 300 });
});

function renderSettings(target: Daemon | null = null) {
  return render(<TaskReminderSettings target={target} />);
}

describe("TaskReminderSettings", () => {
  it("uses the disabled 300-second policy when the daemon has no config key", async () => {
    vi.mocked(getTaskReminderPolicyOn).mockResolvedValue({ enabled: false, idle_threshold_s: 300 });

    renderSettings();

    expect(await screen.findByRole("switch", { name: "Enable task reminders" })).toHaveAttribute("aria-checked", "false");
    expect(screen.getByLabelText("Idle threshold (seconds)")).toHaveValue("300");
    expect(getTaskReminderPolicyOn).toHaveBeenCalledWith(null);
  });

  it("saves the enabled normalized policy", async () => {
    vi.mocked(setTaskReminderPolicyOn).mockResolvedValue({ enabled: true, idle_threshold_s: 120 });
    renderSettings();

    const toggle = await screen.findByRole("switch", { name: "Enable task reminders" });
    fireEvent.click(toggle);
    fireEvent.change(screen.getByLabelText("Idle threshold (seconds)"), { target: { value: "120" } });
    fireEvent.click(screen.getByRole("button", { name: "Save task reminders" }));

    await waitFor(() => expect(setTaskReminderPolicyOn).toHaveBeenCalledWith(null, {
      enabled: true,
      idle_threshold_s: 120,
    }));
    expect(await screen.findByText("Task reminders saved")).toBeInTheDocument();
  });

  it.each(["0", "1.5"])("rejects an invalid threshold of %s before saving", async (value) => {
    renderSettings();

    await screen.findByRole("switch", { name: "Enable task reminders" });
    fireEvent.change(screen.getByLabelText("Idle threshold (seconds)"), { target: { value } });
    fireEvent.click(screen.getByRole("button", { name: "Save task reminders" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a positive whole number of seconds.");
    expect(setTaskReminderPolicyOn).not.toHaveBeenCalled();
  });

  it("keeps the operator draft when the request fails", async () => {
    vi.mocked(setTaskReminderPolicyOn).mockRejectedValue(new Error("daemon unavailable"));
    renderSettings();

    const toggle = await screen.findByRole("switch", { name: "Enable task reminders" });
    fireEvent.click(toggle);
    fireEvent.change(screen.getByLabelText("Idle threshold (seconds)"), { target: { value: "120" } });
    fireEvent.click(screen.getByRole("button", { name: "Save task reminders" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("daemon unavailable");
    expect(toggle).toHaveAttribute("aria-checked", "true");
    expect(screen.getByLabelText("Idle threshold (seconds)")).toHaveValue("120");
  });

  it("targets the explicit server for loading and saving", async () => {
    vi.mocked(getTaskReminderPolicyOn).mockResolvedValue({ enabled: true, idle_threshold_s: 120 });
    renderSettings(remote);

    await screen.findByRole("switch", { name: "Enable task reminders" });
    expect(getTaskReminderPolicyOn).toHaveBeenCalledWith(remote);
    fireEvent.click(screen.getByRole("button", { name: "Save task reminders" }));

    await waitFor(() => expect(setTaskReminderPolicyOn).toHaveBeenCalledWith(remote, {
      enabled: true,
      idle_threshold_s: 120,
    }));
  });
});
