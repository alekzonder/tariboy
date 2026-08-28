import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import PluginSettings from "./PluginSettings";
import {
  getPluginContributionsOn,
  getPluginStatusOn,
  runPluginActionOn,
  type PluginContribution,
} from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  getPluginContributionsOn: vi.fn(),
  getPluginStatusOn: vi.fn(),
  runPluginActionOn: vi.fn(),
}));

const remote: Daemon = {
  id: "remote-1", label: "Production", baseURL: "https://production.example", token: "daemon-token",
};

const telegram: PluginContribution = {
  name: "telegram",
  settings: {
    title: "Telegram",
    status: [
      { name: "token_configured", label: "Token configured" },
      { name: "allowlist_count", label: "Allowed users" },
    ],
    sections: [{
      title: "Bot",
      fields: [
        { name: "token", label: "Bot token", type: "password" },
        { name: "allowed_uids", label: "Allowed UIDs", type: "integer-list", required: true },
      ],
      actions: [{ label: "Save", action: "configure", fields: ["token", "allowed_uids"] }],
    }],
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPluginContributionsOn).mockResolvedValue({ plugins: [telegram], count: 1 });
  vi.mocked(getPluginStatusOn).mockResolvedValue({ token_configured: true, allowlist_count: 2 });
  vi.mocked(runPluginActionOn).mockResolvedValue({ configured: true });
});

it("renders write-only fields and sends normalized action data to the selected server", async () => {
  render(<PluginSettings name="telegram" target={remote} />);

  expect(await screen.findByRole("heading", { name: "Telegram" })).toBeInTheDocument();
  expect(screen.getByText("Token configured: Yes")).toBeInTheDocument();
  expect(screen.getByText("Allowed users: 2")).toBeInTheDocument();
  expect(screen.getByLabelText("Bot token")).toHaveAttribute("type", "password");
  expect(screen.getByLabelText("Bot token")).toHaveValue("");

  fireEvent.change(screen.getByLabelText("Bot token"), { target: { value: "123:secret" } });
  fireEvent.change(screen.getByLabelText("Allowed UIDs"), { target: { value: "22, 11" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(runPluginActionOn).toHaveBeenCalledWith(remote, "telegram", "configure", {
    token: "123:secret",
    allowed_uids: [22, 11],
  }));
  expect(screen.getByLabelText("Bot token")).toHaveValue("");
});

it("rejects a malformed integer list before sending", async () => {
  render(<PluginSettings name="telegram" target={null} />);
  await screen.findByRole("heading", { name: "Telegram" });
  fireEvent.change(screen.getByLabelText("Allowed UIDs"), { target: { value: "11,nope" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Allowed UIDs must be a comma-separated integer list");
  expect(runPluginActionOn).not.toHaveBeenCalled();
});
