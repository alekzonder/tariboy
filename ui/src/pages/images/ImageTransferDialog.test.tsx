import { expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import {
  eligibleImageTransferTargets,
  ImageTransferDialog,
} from "./ImageTransferDialog";

const ready = { id: "ready", label: "Ready", baseURL: "https://ready", state: "ready" } as const;
const offline = { id: "offline", label: "Offline", baseURL: "https://offline", state: "failed" } as const;
const source = { id: "source", label: "Source", baseURL: "https://source", state: "ready", token: "source-token" } as const;

it("keeps ready configured hosts eligible when the source is local", () => {
  expect(eligibleImageTransferTargets(null, [ready, offline])).toEqual([ready]);
});

it("excludes the explicit source and unready hosts", () => {
  expect(eligibleImageTransferTargets(source, [source, ready, offline])).toEqual([ready]);
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

  const selectAll = screen.getByRole("checkbox", { name: "Select all servers" });
  const readyTarget = screen.getByRole("checkbox", { name: "Transfer to Ready" });
  expect(selectAll).not.toBeChecked();
  expect(readyTarget).not.toBeChecked();

  fireEvent.click(selectAll);
  expect(selectAll).toBeChecked();
  expect(readyTarget).toBeChecked();
  expect(screen.queryByRole("checkbox", { name: "Transfer to Offline" })).not.toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: "Transfer to Source" })).not.toBeInTheDocument();

  fireEvent.click(readyTarget);
  expect(selectAll).not.toBeChecked();
  expect(readyTarget).not.toBeChecked();
});

it("disables transfer when no target is eligible", () => {
  render(
    <ImageTransferDialog
      open
      onOpenChange={() => undefined}
      source={source}
      ref="reviewer:v1"
      daemons={[source, offline]}
      onComplete={() => undefined}
    />,
  );

  expect(screen.getByText("No ready servers are available for transfer.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Start transfer" })).toBeDisabled();
});
