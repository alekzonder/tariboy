import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
  DEFAULT_WORKSPACE_ID,
  createWorkspace,
  moveHost,
  resetWorkspacesForTest,
  selectWorkspace,
  useWorkspaces,
  workspaceOf,
} from "@/lib/workspaces";
import { WorkspaceManager, WorkspaceSwitcher, type WorkspaceHost } from "./HostWorkspaces";

const hosts: WorkspaceHost[] = [
  { id: "", label: "This daemon (local)", ready: true },
  { id: "h1", label: "hetzner-01", ready: true },
  { id: "h2", label: "lab-mini", ready: false },
];

beforeEach(() => {
  localStorage.clear();
  resetWorkspacesForTest();
});

function Harness({ onSelect = () => {} }: { onSelect?: (id: string) => void }) {
  const [manager, setManager] = useState<{ id: string; focusNew: boolean } | null>(null);
  const state = useWorkspaces();
  return (
    <>
      <WorkspaceSwitcher
        hosts={hosts}
        onSelect={onSelect}
        onManage={(focusNew) => setManager({ id: state.active, focusNew })}
      />
      <WorkspaceManager
        hosts={hosts}
        open={manager !== null}
        initialId={manager?.id ?? DEFAULT_WORKSPACE_ID}
        focusNew={manager?.focusNew ?? false}
        onOpenChange={(open) => { if (!open) setManager(null); }}
      />
      <output data-testid="active">{state.active}</output>
    </>
  );
}

describe("WorkspaceSwitcher", () => {
  it("lists workspaces with host counts and switches the current one", async () => {
    const lab = createWorkspace("Lab")!;
    moveHost("h1", lab);
    const onSelect = vi.fn();
    render(<Harness onSelect={onSelect} />);

    await userEvent.click(screen.getByRole("button", { name: "Workspace: Default" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByText("Workspaces")).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: /Default\s*2 hosts/ })).toBeInTheDocument();
    await userEvent.click(within(menu).getByRole("menuitem", { name: /Lab\s*1 host/ }));

    expect(screen.getByTestId("active")).toHaveTextContent(lab);
    expect(onSelect).toHaveBeenCalledWith(lab);
    expect(screen.getByRole("button", { name: "Workspace: Lab" })).toBeInTheDocument();
  });

  it("opens the manager focused on a new workspace name", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Workspace: Default" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "New workspace…" }));

    const dialog = await screen.findByRole("dialog", { name: "Manage workspaces" });
    expect(within(dialog).getByPlaceholderText("New workspace name")).toHaveFocus();
  });
});

describe("WorkspaceManager", () => {
  async function openManager() {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: /^Workspace: / }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Manage workspaces…" }));
    return screen.findByRole("dialog", { name: "Manage workspaces" });
  }

  it("shows Default as built-in with its hosts and no Move to while alone", async () => {
    const dialog = await openManager();
    expect(within(dialog).getByText("built-in")).toBeInTheDocument();
    expect(within(dialog).getByText("Can't be renamed or deleted.")).toBeInTheDocument();
    expect(within(dialog).getByText("hetzner-01")).toBeInTheDocument();
    expect(within(dialog).getAllByText("ready")).toHaveLength(2);
    expect(within(dialog).getByText("offline")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: /Move .* to/ })).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Delete workspace" })).toBeNull();
  });

  it("creates a workspace on Enter, selects it and moves a host into it", async () => {
    const dialog = await openManager();
    const name = within(dialog).getByPlaceholderText("New workspace name");
    await userEvent.type(name, "   {Enter}");
    expect(within(dialog).queryByRole("button", { name: /^Production/ })).toBeNull();
    await userEvent.type(name, "Production{Enter}");

    expect(within(dialog).getByLabelText("Workspace name")).toHaveValue("Production");
    expect(within(dialog).getByText("No hosts yet. Open another workspace and move a host here."))
      .toBeInTheDocument();

    await userEvent.click(within(dialog).getByRole("button", { name: /^Default/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Move hetzner-01 to…" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Production" }));

    const state = () => JSON.parse(localStorage.getItem("app:workspaces:v1")!);
    expect(workspaceOf(state(), "h1")).not.toBe(DEFAULT_WORKSPACE_ID);
    expect(within(dialog).queryByText("hetzner-01")).toBeNull();
    expect(within(dialog).getByRole("button", { name: /^Production\s*1$/ })).toBeInTheDocument();
  });

  it("keeps one Move to row open at a time", async () => {
    createWorkspace("Lab");
    const dialog = await openManager();
    await userEvent.click(within(dialog).getByRole("button", { name: "Move hetzner-01 to…" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Move lab-mini to…" }));
    expect(within(dialog).getAllByRole("button", { name: "Lab" })).toHaveLength(1);
  });

  it("renames as it types but never saves a blank name", async () => {
    const lab = createWorkspace("Lab")!;
    selectWorkspace(lab);
    const dialog = await openManager();
    const input = within(dialog).getByLabelText("Workspace name");
    await userEvent.clear(input);
    await userEvent.type(input, "Staging");
    expect(screen.getByRole("button", { name: "Workspace: Staging", hidden: true })).toBeInTheDocument();

    await userEvent.clear(input);
    fireEvent.blur(input);
    expect(input).toHaveValue("Staging");
  });

  it("deletes after an inline confirmation, returning hosts to Default", async () => {
    const lab = createWorkspace("Lab")!;
    moveHost("h1", lab);
    selectWorkspace(lab);
    const dialog = await openManager();

    await userEvent.click(within(dialog).getByRole("button", { name: "Delete workspace" }));
    expect(within(dialog).getByText("Delete Lab? Its hosts move to Default.")).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete workspace" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(screen.getByTestId("active")).toHaveTextContent(DEFAULT_WORKSPACE_ID);
    expect(within(dialog).getByText("built-in")).toBeInTheDocument();
    expect(within(dialog).getByText("hetzner-01")).toBeInTheDocument();
  });
});
