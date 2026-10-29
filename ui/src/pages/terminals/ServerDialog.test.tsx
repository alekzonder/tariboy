import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, renderHook, screen, fireEvent, waitFor } from "@testing-library/react";
import { ServerDialog } from "./ServerDialog";
import { addDaemon, listDaemons, getDaemonToken, type DaemonMeta } from "@/lib/daemons";
import {
  DEFAULT_WORKSPACE_ID,
  createWorkspace,
  inWorkspace,
  resetWorkspacesForTest,
  selectWorkspace,
  useWorkspaces,
} from "@/lib/workspaces";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function seed(token = "tok"): Promise<DaemonMeta> {
  return addDaemon({ label: "prod", baseURL: "http://127.0.0.1:9992", token });
}

function renderDialog(server?: DaemonMeta) {
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(<ServerDialog open server={server} onSaved={onSaved} onOpenChange={onOpenChange} />);
  return { onSaved, onOpenChange };
}

describe("ServerDialog in the Android app", () => {
  afterEach(() => vi.unstubAllEnvs());

  it("adds a forwarded loopback port without a token", async () => {
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    const { onSaved } = renderDialog();
    expect(screen.getByLabelText("base URL")).toHaveAttribute(
      "placeholder",
      "http://127.0.0.1:9990 or https://host:port",
    );
    fireEvent.change(screen.getByLabelText("label"), { target: { value: "fwd" } });
    fireEvent.change(screen.getByLabelText("base URL"), {
      target: { value: "http://127.0.0.1:9990" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const [meta] = await listDaemons();
    expect(meta.baseURL).toBe("http://127.0.0.1:9990");
    expect(await getDaemonToken(meta.id)).toBe("");
  });

  it("rejects plain http to a non-loopback host before saving", async () => {
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    renderDialog();
    fireEvent.change(screen.getByLabelText("label"), { target: { value: "lan" } });
    fireEvent.change(screen.getByLabelText("base URL"), {
      target: { value: "http://192.168.1.5:9990" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(
      screen.getByText(
        "Android allows plain http only to a local forwarded port (http://127.0.0.1:PORT); use https for other hosts",
      ),
    ).toBeInTheDocument();
    expect(await listDaemons()).toHaveLength(0);
  });
});

describe("ServerDialog", () => {
  it("adds a new server to the current workspace only", async () => {
    resetWorkspacesForTest();
    const lab = createWorkspace("Lab")!;
    selectWorkspace(lab);
    const { onSaved } = renderDialog();
    fireEvent.change(screen.getByLabelText("label"), { target: { value: "prod" } });
    fireEvent.change(screen.getByLabelText("base URL"), { target: { value: "http://h:9990" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const [meta] = await listDaemons();
    const state = renderHook(() => useWorkspaces()).result.current;
    expect(inWorkspace(state, meta.id, lab)).toBe(true);
    expect(inWorkspace(state, meta.id, DEFAULT_WORKSPACE_ID)).toBe(false);
  });

  it("adds a new server with the typed token", async () => {
    const { onSaved, onOpenChange } = renderDialog();
    expect(screen.getByText("Add host")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("label"), { target: { value: "  prod  " } });
    fireEvent.change(screen.getByLabelText("base URL"), { target: { value: "http://h:9990/" } });
    fireEvent.change(screen.getByLabelText("token"), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const [meta] = await listDaemons();
    expect(meta.label).toBe("prod");
    expect(meta.baseURL).toBe("http://h:9990");
    expect(await getDaemonToken(meta.id)).toBe("secret");
    expect(onSaved).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("rejects an empty label and a non-http base URL", async () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(screen.getByText("label is required")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("label"), { target: { value: "prod" } });
    fireEvent.change(screen.getByLabelText("base URL"), { target: { value: "h:9990" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(screen.getByText("base URL must start with http")).toBeInTheDocument();
    expect(await listDaemons()).toHaveLength(0);
  });

  it("seeds the form from the edited server and never renders the token back", async () => {
    const meta = await seed();
    renderDialog(meta);

    expect(screen.getByText("Edit host")).toBeInTheDocument();
    await waitFor(() =>
      expect((screen.getByLabelText("label") as HTMLInputElement).value).toBe("prod"),
    );
    expect((screen.getByLabelText("base URL") as HTMLInputElement).value).toBe(
      "http://127.0.0.1:9992",
    );
    expect((screen.getByLabelText("token") as HTMLInputElement).value).toBe("");
    expect(await screen.findByText("token set for this session")).toBeInTheDocument();
  });

  it("keeps the stored token when the token field is left blank", async () => {
    const meta = await seed();
    const { onSaved } = renderDialog(meta);

    await waitFor(() =>
      expect((screen.getByLabelText("label") as HTMLInputElement).value).toBe("prod"),
    );
    fireEvent.change(screen.getByLabelText("label"), { target: { value: "renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const saved = await listDaemons();
    expect(saved).toHaveLength(1);
    expect(saved[0].id).toBe(meta.id);
    expect(saved[0].label).toBe("renamed");
    expect(await getDaemonToken(meta.id)).toBe("tok");
  });

  it("overwrites the token when one is typed", async () => {
    const meta = await seed();
    const { onSaved } = renderDialog(meta);

    await waitFor(() =>
      expect((screen.getByLabelText("label") as HTMLInputElement).value).toBe("prod"),
    );
    fireEvent.change(screen.getByLabelText("token"), { target: { value: "fresh" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(await getDaemonToken(meta.id)).toBe("fresh");
  });

  it("clears the token on demand and reports it as unset", async () => {
    const meta = await seed();
    const { onSaved } = renderDialog(meta);

    fireEvent.click(await screen.findByRole("button", { name: "Clear token" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(await getDaemonToken(meta.id)).toBe("");
    expect(await screen.findByText("no token set")).toBeInTheDocument();
    // Clearing must not drop the registry entry itself.
    expect(await listDaemons()).toHaveLength(1);
  });

  it("surfaces credential-store errors while opening the editor", async () => {
    const meta = await seed();
    const getItem = Storage.prototype.getItem;
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (
      this: Storage,
      key: string,
    ) {
      if (this === sessionStorage && key.endsWith(meta.id))
        throw new Error("credential store unavailable");
      return getItem.call(this, key);
    });

    renderDialog(meta);

    expect(await screen.findByText(/credential store unavailable/)).toBeInTheDocument();
  });
});
