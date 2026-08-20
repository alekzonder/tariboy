import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as imageTransferApi from "@/lib/teamApi";
import { ApiError } from "@/lib/api";
import { useState } from "react";
import {
  eligibleImageTransferTargets,
  ImageTransferDialog,
} from "./ImageTransferDialog";

vi.mock("@/lib/teamApi", () => ({
  downloadImageArchiveOn: vi.fn(),
  uploadImageArchiveOn: vi.fn(),
  applyImageArchiveOn: vi.fn(),
}));

const downloadImageArchiveOn = vi.mocked(imageTransferApi.downloadImageArchiveOn);
const uploadImageArchiveOn = vi.mocked(imageTransferApi.uploadImageArchiveOn);
const applyImageArchiveOn = vi.mocked(imageTransferApi.applyImageArchiveOn);

afterEach(() => vi.resetAllMocks());

const ready = { id: "ready", label: "Ready", baseURL: "https://ready", state: "ready", token: "ready-token" } as const;
const offline = { id: "offline", label: "Offline", baseURL: "https://offline", state: "failed", token: "offline-token" } as const;
const source = { id: "source", label: "Source", baseURL: "https://source", state: "ready", token: "source-token" } as const;

if (false) {
  // @ts-expect-error Image transfer must never fall back to the active daemon.
  eligibleImageTransferTargets(undefined, [ready]);
}

it("keeps ready configured hosts eligible when the source is local", () => {
  expect(eligibleImageTransferTargets(null, [ready, offline])).toEqual([
    { id: ready.id, label: ready.label, target: ready },
  ]);
});

it("includes the implicit local target once for a remote source", () => {
  expect(eligibleImageTransferTargets(source, [source, ready, offline])).toEqual([
    { id: "", label: "This daemon (local)", target: null },
    { id: ready.id, label: ready.label, target: ready },
  ]);
});

it("selects every eligible target and permits individual deselection", () => {
  render(
    <ImageTransferDialog
      open
      onOpenChange={() => undefined}
      source={source}
      ref="reviewer:v1"
      daemons={[source, ready, offline]}
      onComplete={() => undefined}
    />,
  );

  const selectAll = screen.getByRole("button", { name: "All servers" });
  const readyTarget = screen.getByRole("checkbox", { name: "Transfer to Ready" });
  expect(readyTarget).not.toBeChecked();

  fireEvent.click(selectAll);
  expect(screen.getByRole("button", { name: "Clear all servers" })).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: "Transfer to This daemon (local)" })).toBeChecked();
  expect(readyTarget).toBeChecked();
  expect(screen.queryByRole("checkbox", { name: "Transfer to Offline" })).not.toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: "Transfer to Source" })).not.toBeInTheDocument();

  fireEvent.click(readyTarget);
  expect(screen.getByRole("button", { name: "All servers" })).toBeInTheDocument();
  expect(readyTarget).not.toBeChecked();
});

it("disables transfer when no target is eligible", () => {
  render(
    <ImageTransferDialog
      open
      onOpenChange={() => undefined}
      source={null}
      ref="reviewer:v1"
      daemons={[offline]}
      onComplete={() => undefined}
    />,
  );

  expect(screen.getByText("No ready servers are available for transfer.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Start transfer" })).toBeDisabled();
});

it("exports once, continues after a failed target, and reports an idempotent target", async () => {
  const targetA = { id: "target-a", label: "Target A", baseURL: "https://a", state: "ready", token: "a-token" } as const;
  const targetB = { id: "target-b", label: "Target B", baseURL: "https://b", state: "ready", token: "b-token" } as const;
  const archive = new Blob(["archive"]);
  downloadImageArchiveOn.mockResolvedValue(archive);
  uploadImageArchiveOn
    .mockResolvedValueOnce({ import_id: "import-a", ref: "reviewer:v3", digest: "a" })
    .mockResolvedValueOnce({ import_id: "import-b", ref: "reviewer:v3", digest: "b" });
  applyImageArchiveOn
    .mockRejectedValueOnce(new Error("target A failed"))
    .mockResolvedValueOnce({ reused: true });
  const user = userEvent.setup();

  render(
    <ImageTransferDialog
      open
      onOpenChange={() => undefined}
      source={null}
      ref="reviewer:v3"
      daemons={[targetA, targetB]}
      onComplete={() => undefined}
    />,
  );

  await user.click(screen.getByRole("button", { name: "All servers" }));
  await user.click(screen.getByRole("button", { name: "Start transfer" }));

  await waitFor(() => expect(downloadImageArchiveOn).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(applyImageArchiveOn).toHaveBeenCalledTimes(2));
  expect(downloadImageArchiveOn).toHaveBeenCalledWith(null, "reviewer:v3");
  expect(uploadImageArchiveOn).toHaveBeenNthCalledWith(1, targetA, archive);
  expect(uploadImageArchiveOn).toHaveBeenNthCalledWith(2, targetB, archive);
  expect(applyImageArchiveOn).toHaveBeenNthCalledWith(2, targetB, "import-b", "reviewer:v3");
  expect(screen.getByText("Target A: Failed — target A failed")).toBeInTheDocument();
  expect(screen.getByText("Target B: Already present")).toBeInTheDocument();
});

it("cancels before starting the next selected target", async () => {
  const targetA = { id: "target-a", label: "Target A", baseURL: "https://a", state: "ready", token: "a-token" } as const;
  const targetB = { id: "target-b", label: "Target B", baseURL: "https://b", state: "ready", token: "b-token" } as const;
  const archive = new Blob(["archive"]);
  let finishFirstApply: () => void = () => undefined;
  downloadImageArchiveOn.mockResolvedValue(archive);
  uploadImageArchiveOn.mockResolvedValueOnce({ import_id: "import-a", ref: "reviewer:v3", digest: "a" });
  applyImageArchiveOn.mockImplementationOnce(() => new Promise<void>((resolve) => { finishFirstApply = resolve; }));
  const user = userEvent.setup();

  render(<ImageTransferDialog open onOpenChange={() => undefined} source={null} ref="reviewer:v3" daemons={[targetA, targetB]} onComplete={() => undefined} />);
  await user.click(screen.getByRole("button", { name: "All servers" }));
  await user.click(screen.getByRole("button", { name: "Start transfer" }));
  await waitFor(() => expect(applyImageArchiveOn).toHaveBeenCalledTimes(1));

  await user.click(screen.getByRole("button", { name: "Cancel transfer" }));
  finishFirstApply();

  await waitFor(() => expect(screen.getByText("Target B: Cancelled")).toBeInTheDocument());
  expect(uploadImageArchiveOn).toHaveBeenCalledTimes(1);
  expect(screen.getByText("Target A: Completed")).toBeInTheDocument();
});

it("retags and retries only the conflicted target without another export", async () => {
  const target = { id: "target-a", label: "Target A", baseURL: "https://a", state: "ready", token: "a-token" } as const;
  const archive = new Blob(["archive"]);
  downloadImageArchiveOn.mockResolvedValue(archive);
  uploadImageArchiveOn
    .mockResolvedValueOnce({ import_id: "import-a", ref: "reviewer:v3", digest: "a" })
    .mockResolvedValueOnce({ import_id: "import-b", ref: "reviewer-copy:v4", digest: "b" });
  applyImageArchiveOn
    .mockRejectedValueOnce(new ApiError(409, "image_import_failed", "original ref conflicts"))
    .mockResolvedValueOnce({ reused: false });
  const user = userEvent.setup();

  render(<ImageTransferDialog open onOpenChange={() => undefined} source={null} ref="reviewer:v3" daemons={[target]} onComplete={() => undefined} />);
  await user.click(screen.getByRole("button", { name: "All servers" }));
  await user.click(screen.getByRole("button", { name: "Start transfer" }));
  const retryRef = await screen.findByRole("textbox", { name: "Retag and retry for Target A" });
  expect(retryRef).toHaveValue("reviewer:v3");

  await user.clear(retryRef);
  await user.type(retryRef, "reviewer-copy:v4");
  await user.click(screen.getByRole("button", { name: "Retag and retry Target A" }));

  await waitFor(() => expect(applyImageArchiveOn).toHaveBeenCalledTimes(2));
  expect(downloadImageArchiveOn).toHaveBeenCalledTimes(1);
  expect(uploadImageArchiveOn).toHaveBeenCalledTimes(2);
  expect(uploadImageArchiveOn).toHaveBeenNthCalledWith(2, target, archive);
  expect(applyImageArchiveOn).toHaveBeenNthCalledWith(2, target, "import-b", "reviewer-copy:v4");
  expect(screen.getByText("Target A: Completed")).toBeInTheDocument();
});

it("clears staged retry state when the dialog closes", async () => {
  const target = { id: "target-a", label: "Target A", baseURL: "https://a", state: "ready", token: "a-token" } as const;
  downloadImageArchiveOn.mockResolvedValue(new Blob(["archive"]));
  uploadImageArchiveOn.mockResolvedValue({ import_id: "import-a", ref: "reviewer:v3", digest: "a" });
  applyImageArchiveOn.mockRejectedValue(new ApiError(409, "image_import_failed", "original ref conflicts"));
  const user = userEvent.setup();
  const Harness = () => {
    const [open, setOpen] = useState(true);
    return <><button onClick={() => setOpen(true)}>Reopen</button><ImageTransferDialog open={open} onOpenChange={setOpen} source={null} ref="reviewer:v3" daemons={[target]} onComplete={() => undefined} /></>;
  };

  render(<Harness />);
  await user.click(screen.getByRole("button", { name: "All servers" }));
  await user.click(screen.getByRole("button", { name: "Start transfer" }));
  expect(await screen.findByRole("textbox", { name: "Retag and retry for Target A" })).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Close" }));
  await user.click(screen.getByRole("button", { name: "Reopen" }));

  expect(screen.queryByRole("textbox", { name: "Retag and retry for Target A" })).not.toBeInTheDocument();
});

it("cancels an in-flight transfer when its parent closes the dialog", async () => {
  const targetA = { id: "target-a", label: "Target A", baseURL: "https://a", state: "ready", token: "a-token" } as const;
  const targetB = { id: "target-b", label: "Target B", baseURL: "https://b", state: "ready", token: "b-token" } as const;
  let finishFirstApply: () => void = () => undefined;
  downloadImageArchiveOn.mockResolvedValue(new Blob(["archive"]));
  uploadImageArchiveOn
    .mockResolvedValueOnce({ import_id: "import-a", ref: "reviewer:v3", digest: "a" })
    .mockResolvedValueOnce({ import_id: "import-b", ref: "reviewer:v3", digest: "b" });
  applyImageArchiveOn.mockImplementationOnce(() => new Promise<void>((resolve) => { finishFirstApply = resolve; }));
  const user = userEvent.setup();
  const Harness = () => {
    const [open, setOpen] = useState(true);
    return <>
      <button data-testid="parent-close" onClick={() => setOpen(false)}>Parent close</button>
      <button data-testid="parent-reopen" onClick={() => setOpen(true)}>Parent reopen</button>
      <ImageTransferDialog open={open} onOpenChange={setOpen} source={null} ref="reviewer:v3" daemons={[targetA, targetB]} onComplete={() => undefined} />
    </>;
  };

  render(<Harness />);
  await user.click(screen.getByRole("button", { name: "All servers" }));
  await user.click(screen.getByRole("button", { name: "Start transfer" }));
  await waitFor(() => expect(applyImageArchiveOn).toHaveBeenCalledTimes(1));

  fireEvent.click(screen.getByTestId("parent-close"));
  finishFirstApply();
  await waitFor(() => expect(uploadImageArchiveOn).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByTestId("parent-reopen"));

  expect(screen.queryByText("Target A: Completed")).not.toBeInTheDocument();
  expect(screen.queryByText("Target B: Queued")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Start transfer" })).toBeDisabled();
});
