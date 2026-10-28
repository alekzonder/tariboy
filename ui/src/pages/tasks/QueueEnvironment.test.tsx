import type { ReactElement } from "react";
import { fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import QueueEnvironment from "./QueueEnvironment";
import { developmentManifest, remoteTarget } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({
  listQueueEnv: vi.fn(),
  setQueueEnv: vi.fn(),
  removeQueueEnv: vi.fn(),
  listQueueSecrets: vi.fn(),
  setQueueSecret: vi.fn(),
  removeQueueSecret: vi.fn(),
}));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  {
    workflows: {
      listQueueEnv: api.listQueueEnv,
      setQueueEnv: api.setQueueEnv,
      removeQueueEnv: api.removeQueueEnv,
      listQueueSecrets: api.listQueueSecrets,
      setQueueSecret: api.setQueueSecret,
      removeQueueSecret: api.removeQueueSecret,
    },
  },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });

const env = () => screen.getByRole("region", { name: "Env REL" });
const fillEnv = (key: string, value: string) => {
  fireEvent.change(screen.getByLabelText("Env key"), { target: { value: key } });
  fireEvent.change(screen.getByLabelText("Env value"), { target: { value } });
};

beforeEach(() => {
  vi.resetAllMocks();
  api.listQueueEnv.mockResolvedValue([
    { key: "LOG_LEVEL", value: "debug", updated_at: "2026-10-02T10:00:00Z" },
  ]);
  api.listQueueSecrets.mockResolvedValue({
    secrets: [{ key: "TOKEN", updated_at: "2026-10-02T10:00:00Z" }],
    global_env: [],
  });
});

it("shows env vars with their values apart from the secrets", async () => {
  render(<QueueEnvironment queue="REL" />);
  expect(await within(env()).findByText("LOG_LEVEL")).toBeInTheDocument();
  expect(within(env()).getByText("debug")).toBeInTheDocument();
  expect(within(env()).queryByText("TOKEN")).not.toBeInTheDocument();
  expect(
    within(screen.getByRole("region", { name: "Secrets REL" })).getByText("TOKEN"),
  ).toBeInTheDocument();
  expect(screen.getByLabelText("Env value")).not.toHaveAttribute("type", "password");
});

it("sets an env var and reloads both blocks", async () => {
  api.setQueueEnv.mockResolvedValue({
    queue: "REL",
    key: "BASE_URL",
    value: "https://x.test",
    updated_at: "t",
  });
  render(<QueueEnvironment queue="REL" />);
  await within(env()).findByText("LOG_LEVEL");
  fillEnv("BASE_URL", "https://x.test");
  fireEvent.click(within(env()).getByRole("button", { name: "Set" }));
  await waitFor(() =>
    expect(api.setQueueEnv).toHaveBeenCalledWith("REL", "BASE_URL", "https://x.test"),
  );
  await waitFor(() => expect(api.listQueueSecrets).toHaveBeenCalledTimes(2));
  expect(api.listQueueEnv).toHaveBeenCalledTimes(2);
  expect(screen.getByLabelText("Env key")).toHaveValue("");
});

it("asks before moving a secret key into env", async () => {
  api.setQueueEnv.mockResolvedValue({
    queue: "REL",
    key: "TOKEN",
    value: "plain",
    updated_at: "t",
  });
  render(<QueueEnvironment queue="REL" />);
  await screen.findByText("TOKEN");
  fillEnv("TOKEN", "plain");
  fireEvent.click(within(env()).getByRole("button", { name: "Set" }));
  fireEvent.click(await screen.findByRole("button", { name: "Go back" }));
  expect(api.setQueueEnv).not.toHaveBeenCalled();
  fireEvent.click(within(env()).getByRole("button", { name: "Set" }));
  fireEvent.click(await screen.findByRole("button", { name: "Move to env" }));
  await waitFor(() => expect(api.setQueueEnv).toHaveBeenCalledWith("REL", "TOKEN", "plain"));
});

it("asks before moving an env key into secrets", async () => {
  api.setQueueSecret.mockResolvedValue({ queue: "REL", key: "LOG_LEVEL", updated_at: "t" });
  render(<QueueEnvironment queue="REL" />);
  await screen.findByText("LOG_LEVEL");
  fireEvent.change(screen.getByLabelText("Secret key"), { target: { value: "LOG_LEVEL" } });
  fireEvent.change(screen.getByLabelText("Secret value"), { target: { value: "s3cr3t" } });
  fireEvent.click(
    within(screen.getByRole("region", { name: "Secrets REL" })).getByRole("button", {
      name: "Set",
    }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "Move to secrets" }));
  await waitFor(() =>
    expect(api.setQueueSecret).toHaveBeenCalledWith("REL", "LOG_LEVEL", "s3cr3t"),
  );
  expect(screen.getByLabelText("Secret value")).toHaveValue("");
});

it("removes an env var after a confirmation and shows a refusal inline", async () => {
  api.removeQueueEnv.mockRejectedValue(
    new ApiError(409, "workflow_secret_missing", "the bound workflow requires LOG_LEVEL"),
  );
  render(<QueueEnvironment queue="REL" />);
  await screen.findByText("LOG_LEVEL");
  fireEvent.click(screen.getByRole("button", { name: "Remove LOG_LEVEL" }));
  fireEvent.click(await screen.findByRole("button", { name: "Remove env var" }));
  await waitFor(() => expect(api.removeQueueEnv).toHaveBeenCalledWith("REL", "LOG_LEVEL"));
  expect(await within(env()).findByRole("alert")).toHaveTextContent("requires LOG_LEVEL");
});

it("counts an env var as a value for a name a bind needs", async () => {
  render(<QueueEnvironment queue="REL" missing={["LOG_LEVEL", "OTHER"]} />);
  await screen.findByText("LOG_LEVEL");
  await waitFor(() =>
    expect(screen.getByRole("alert", { name: "Secrets needed" })).not.toHaveTextContent(
      "LOG_LEVEL",
    ),
  );
  expect(screen.getByRole("alert", { name: "Secrets needed" })).toHaveTextContent("OTHER");
});

const declared = {
  ...developmentManifest.definition,
  env: { LOG_LEVEL: "info", TIMEOUT: "30" },
  requires_secrets: ["TOKEN", "GH_TOKEN", "LOG_LEVEL"],
};
const chips = (block: string) =>
  screen.queryByRole("group", { name: `${block} declared in Workflowfile` });
const options = (input: HTMLElement) =>
  [
    ...document.querySelectorAll(`datalist#${CSS.escape(input.getAttribute("list") ?? "")} option`),
  ].map((option) => option.getAttribute("value"));

it("suggests the names the workflow declares and offers a chip for each unset one", async () => {
  render(<QueueEnvironment queue="REL" definition={declared} />);
  await within(env()).findByText("LOG_LEVEL");
  await screen.findByText("TOKEN");
  expect(options(screen.getByLabelText("Env key"))).toEqual(["LOG_LEVEL", "TIMEOUT"]);
  expect(options(screen.getByLabelText("Secret key"))).toEqual(["GH_TOKEN", "LOG_LEVEL", "TOKEN"]);
  const envChips = within(chips("Env")!).getAllByRole("button");
  expect(envChips.map((chip) => chip.textContent)).toEqual(["TIMEOUT"]);
  expect(envChips[0]).toHaveAttribute("title", "default: 30");
  // LOG_LEVEL has a value as an env var, so the Secrets block offers no chip for it either.
  expect(
    within(chips("Secrets")!)
      .getAllByRole("button")
      .map((chip) => chip.textContent),
  ).toEqual(["GH_TOKEN"]);
});

it("fills the key from a chip and moves the focus to the value", async () => {
  render(<QueueEnvironment queue="REL" definition={declared} />);
  await within(env()).findByText("LOG_LEVEL");
  fireEvent.click(within(chips("Env")!).getByRole("button", { name: "TIMEOUT" }));
  expect(screen.getByLabelText("Env key")).toHaveValue("TIMEOUT");
  expect(screen.getByLabelText("Env value")).toHaveFocus();
  await screen.findByText("TOKEN");
  fireEvent.click(within(chips("Secrets")!).getByRole("button", { name: "GH_TOKEN" }));
  expect(screen.getByLabelText("Secret key")).toHaveValue("GH_TOKEN");
  expect(screen.getByLabelText("Secret value")).toHaveFocus();
});

it("shows no hints without a bound workflow", async () => {
  render(<QueueEnvironment queue="REL" />);
  await within(env()).findByText("LOG_LEVEL");
  expect(chips("Env")).not.toBeInTheDocument();
  expect(chips("Secrets")).not.toBeInTheDocument();
  expect(screen.getByLabelText("Env key")).not.toHaveAttribute("list");
});
