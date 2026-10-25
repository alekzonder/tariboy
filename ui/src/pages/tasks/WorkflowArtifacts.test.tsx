import type { ReactElement } from "react";
import { act, fireEvent, render as renderWith, screen, within } from "@/test/queryRender";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError, type Artifact } from "@/lib/api";
import WorkflowArtifacts from "./WorkflowArtifacts";
import { remoteTarget } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({ setTaskArtifact: vi.fn(), getTaskArtifactHistory: vi.fn() }));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  {
    workflows: {
      setTaskArtifact: api.setTaskArtifact,
      getTaskArtifactHistory: api.getTaskArtifactHistory,
    },
  },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });

const notes: Artifact = {
  id: 3,
  name: "notes",
  value: "<i>one</i>\n2\n3\n4\n5\n6\n7\n8",
  author: "agent:writer",
  created_at: "2026-10-01T10:04:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
});

function renderArtifacts(props: Partial<Parameters<typeof WorkflowArtifacts>[0]> = {}) {
  const onChanged = vi.fn();
  render(
    <WorkflowArtifacts
      taskKey="REL-1"
      artifacts={[notes]}
      declared={[
        { name: "notes", description: "The notes." },
        { name: "summary", description: "One paragraph." },
      ]}
      editable
      onChanged={onChanged}
      {...props}
    />,
  );
  return { onChanged };
}

it("lists every declared artifact with no value yet, with its description and a Set action", () => {
  renderArtifacts();
  const item = screen.getByText("summary").closest("li")!;
  expect(within(item).getByText("no value")).toBeInTheDocument();
  expect(within(item).getByText("One paragraph.")).toBeInTheDocument();
  expect(within(item).getByRole("button", { name: "Set" })).toBeInTheDocument();
  expect(screen.getAllByRole("listitem")).toHaveLength(2);
});

it("lists every artifact in one list, the ones the current status needs first, missing before set", () => {
  const required = new Map([
    ["notes", [{ on: "drafted", to: "review", missing: false }]],
    ["summary", [{ on: "drafted", to: "review", missing: true }]],
  ]);
  renderArtifacts({
    required,
    declared: [
      { name: "changelog", description: "" },
      { name: "notes", description: "The notes." },
      { name: "summary", description: "One paragraph." },
    ],
  });
  const all = within(screen.getByRole("list", { name: "All artifacts" })).getAllByRole("listitem");
  expect(all.map((item) => item.querySelector("span")!.textContent)).toEqual([
    "summary",
    "notes",
    "changelog",
  ]);
  expect(within(all[0]).getByText("required by drafted → review")).toHaveAttribute(
    "data-requirement",
    "missing",
  );
  expect(within(all[1]).getByText("required by drafted → review")).toHaveAttribute(
    "data-requirement",
    "set",
  );
  expect(within(all[1]).getByText("The notes.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /Other artifacts/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Show all artifacts" })).not.toBeInTheDocument();
});

it("shows only what a selected step needs, by the way in and the way out, and resets to all", () => {
  const onShowAll = vi.fn();
  const step = {
    status: "review",
    enter: new Map([
      ["notes", [{ from: "draft", on: "drafted", to: "review", missing: false }]],
      ["summary", [{ from: "draft", on: "drafted", to: "review", missing: true }]],
    ]),
    leave: new Map([
      ["changelog", [{ from: "review", on: "approve", to: "publish", missing: true }]],
    ]),
  };
  renderArtifacts({
    step,
    onShowAll,
    declared: [
      { name: "notes", description: "The notes." },
      { name: "summary", description: "" },
      { name: "changelog", description: "" },
      { name: "extra", description: "" },
    ],
  });
  expect(screen.getByTestId("artifact-filter")).toHaveTextContent("Step review · 3 of 4 artifacts");
  expect(screen.queryByText("extra")).not.toBeInTheDocument();
  const enter = within(screen.getByRole("list", { name: "Needed to enter review" })).getAllByRole(
    "listitem",
  );
  expect(enter.map((item) => item.querySelector("span")!.textContent)).toEqual([
    "summary",
    "notes",
  ]);
  expect(within(enter[0]).getByText("required by draft · drafted → review")).toHaveAttribute(
    "data-requirement",
    "missing",
  );
  const leave = within(screen.getByRole("list", { name: "Needed to leave review" })).getAllByRole(
    "listitem",
  );
  expect(leave.map((item) => item.querySelector("span")!.textContent)).toEqual(["changelog"]);
  expect(within(leave[0]).getByText("required by review · approve → publish")).toHaveAttribute(
    "data-requirement",
    "missing",
  );
  fireEvent.click(screen.getByRole("button", { name: "Show all artifacts" }));
  expect(onShowAll).toHaveBeenCalledTimes(1);
});

it("shows an artifact needed both ways once, under the way in, with both labels", () => {
  const step = {
    status: "review",
    enter: new Map([["notes", [{ from: "draft", on: "drafted", to: "review", missing: false }]]]),
    leave: new Map([["notes", [{ from: "review", on: "approve", to: "publish", missing: false }]]]),
  };
  renderArtifacts({ step, onShowAll: vi.fn() });
  expect(screen.getAllByRole("listitem")).toHaveLength(1);
  const notes = within(screen.getByRole("list", { name: "Needed to enter review" })).getByRole(
    "listitem",
  );
  expect(within(notes).getByText("required by draft · drafted → review")).toBeInTheDocument();
  expect(within(notes).getByText("required by review · approve → publish")).toBeInTheDocument();
  expect(screen.queryByRole("list", { name: "Needed to leave review" })).not.toBeInTheDocument();
});

it("says so when a selected step needs no artifact", () => {
  renderArtifacts({
    step: { status: "done", enter: new Map(), leave: new Map() },
    onShowAll: vi.fn(),
  });
  expect(screen.getByText("No artifact is required to enter or leave done.")).toBeInTheDocument();
  expect(screen.queryByRole("listitem")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Show all artifacts" })).toBeInTheDocument();
});

it("lets a long author wrap", () => {
  const author = "agent:" + "a".repeat(64);
  renderArtifacts({ artifacts: [{ ...notes, author }] });
  expect(screen.getByText(author)).toHaveClass("min-w-0", "break-all");
});

it("shows each artifact with its author and a long value collapsed to six lines, as Markdown", () => {
  renderArtifacts({
    artifacts: [{ ...notes, value: "# Title\n\n**bold**\n4\n5\n6\nseventh\neighth" }],
  });
  const item = screen.getByText("notes").closest("li")!;
  expect(within(item).getByText("agent:writer")).toBeInTheDocument();
  expect(within(item).getByRole("heading", { name: "Title" })).toBeInTheDocument();
  expect(within(item).getByText("bold").tagName).toBe("STRONG");
  expect(within(item).queryByText(/eighth/)).not.toBeInTheDocument();
  const expand = within(item).getByRole("button", { name: "Show all 8 lines" });
  expect(expand).toHaveAttribute("aria-expanded", "false");
  fireEvent.click(expand);
  expect(within(item).getByText(/eighth/)).toBeInTheDocument();
  expect(within(item).getByRole("button", { name: "Show less" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
});

it("switches a value to plain text, which never interprets Markdown or HTML", () => {
  renderArtifacts();
  const item = screen.getByText("notes").closest("li")!;
  const text = within(item).getByRole("button", { name: "Text" });
  expect(text).toHaveAttribute("aria-pressed", "false");
  fireEvent.click(text);
  expect(text).toHaveAttribute("aria-pressed", "true");
  const value = item.querySelector("pre")!;
  expect(value.textContent).toBe("<i>one</i>\n2\n3\n4\n5\n6");
  expect(value.querySelector("i")).toBeNull();
  fireEvent.click(within(item).getByRole("button", { name: "Show all 8 lines" }));
  expect(item.querySelector("pre")!.textContent).toBe(notes.value);
});

it("loads the history only when asked", async () => {
  api.getTaskArtifactHistory.mockResolvedValue([
    notes,
    { ...notes, id: 2, value: "older value", author: "user:owner" },
  ]);
  renderArtifacts();
  expect(api.getTaskArtifactHistory).not.toHaveBeenCalled();
  const item = screen.getByText("notes").closest("li")!;
  await act(async () => {
    fireEvent.click(within(item).getByRole("button", { name: "History" }));
  });
  expect(api.getTaskArtifactHistory).toHaveBeenCalledWith("REL-1", "notes");
  expect(within(item).getByText("older value")).toBeInTheDocument();
});

it("saves an edited value and reports the change", async () => {
  api.setTaskArtifact.mockResolvedValue({ ...notes, value: "new" });
  const { onChanged } = renderArtifacts();
  const item = screen.getByText("notes").closest("li")!;
  fireEvent.click(within(item).getByRole("button", { name: "Edit" }));
  fireEvent.change(within(item).getByLabelText("Value of notes"), { target: { value: "new" } });
  await act(async () => {
    fireEvent.click(within(item).getByRole("button", { name: "Save" }));
  });
  expect(api.setTaskArtifact).toHaveBeenCalledWith("REL-1", "notes", "new");
  expect(onChanged).toHaveBeenCalled();
  expect(within(item).queryByLabelText("Value of notes")).not.toBeInTheDocument();
});

it("keeps the editor and shows the daemon's error inline", async () => {
  api.setTaskArtifact.mockRejectedValue(
    new ApiError(413, "artifact_too_large", "the value exceeds 64 KiB"),
  );
  const { onChanged } = renderArtifacts();
  const item = screen.getByText("summary").closest("li")!;
  fireEvent.click(within(item).getByRole("button", { name: "Set" }));
  fireEvent.change(within(item).getByLabelText("Value of summary"), { target: { value: "x" } });
  await act(async () => {
    fireEvent.click(within(item).getByRole("button", { name: "Save" }));
  });
  expect(within(item).getByRole("alert")).toHaveTextContent("the value exceeds 64 KiB");
  expect(within(item).getByLabelText("Value of summary")).toBeInTheDocument();
  expect(onChanged).not.toHaveBeenCalled();
});

it("offers no edit action when the task cannot be edited", () => {
  renderArtifacts({ editable: false });
  expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Set" })).not.toBeInTheDocument();
});

it("collapses one very long line by length and shows the whole value, unclipped, once expanded", async () => {
  const long = "x".repeat(5000);
  api.getTaskArtifactHistory.mockResolvedValue([{ ...notes, id: 2, value: long }]);
  renderArtifacts({ artifacts: [{ ...notes, value: long }] });
  const item = screen.getByText("notes").closest("li")!;
  fireEvent.click(within(item).getByRole("button", { name: "Text" }));
  expect(item.querySelector("pre")!.textContent).toHaveLength(600);
  fireEvent.click(within(item).getByRole("button", { name: "Show all 5000 characters" }));
  const value = item.querySelector("pre")!;
  expect(value.textContent).toBe(long);
  expect(value).not.toHaveClass("max-h-60");
  expect(value).not.toHaveClass("overflow-auto");
  await act(async () => {
    fireEvent.click(within(item).getByRole("button", { name: "History" }));
  });
  expect(item.querySelector("ol pre")).not.toBeNull();
});
