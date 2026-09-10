import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ShellScriptEditor } from "./ShellScriptEditor";

it.each(["success", "failure"])("blocks overlapping saves and unlocks after %s", async (outcome) => {
  let resolveSave!: () => void;
  let rejectSave!: (error: Error) => void;
  const save = vi.fn().mockImplementationOnce(() => new Promise<void>((resolve, reject) => {
    resolveSave = resolve;
    rejectSave = reject;
  })).mockResolvedValue({ saved: true });
  render(<ShellScriptEditor title="Script" description="Bash" load={async () => "original"} save={save} />);
  const script = screen.getByLabelText("Script");
  await waitFor(() => expect(script).toBeEnabled());
  const button = screen.getByRole("button", { name: "Save script" });
  fireEvent.change(script, { target: { value: "first" } });
  act(() => {
    button.click();
    button.click();
  });
  expect(save).toHaveBeenCalledTimes(1);
  expect(button).toBeDisabled();
  fireEvent.change(script, { target: { value: "second" } });
  fireEvent.click(button);
  expect(save).toHaveBeenCalledTimes(1);
  await act(async () => {
    if (outcome === "success") resolveSave();
    else rejectSave(new Error("save failed"));
  });
  expect(button).toBeEnabled();
  expect(script).toHaveValue("second");
  if (outcome === "failure") expect(screen.getByRole("alert")).toHaveTextContent("save failed");
  fireEvent.click(screen.getByRole("button", { name: "Discard changes" }));
  expect(script).toHaveValue(outcome === "success" ? "first" : "original");
  fireEvent.change(script, { target: { value: "second" } });
  fireEvent.click(button);
  await waitFor(() => expect(button).toBeEnabled());
  expect(save).toHaveBeenCalledTimes(2);
  expect(save).toHaveBeenLastCalledWith("second");
  expect(screen.queryByRole("button", { name: "Discard changes" })).not.toBeInTheDocument();
});

it("disables discard while a different script is loading", async () => {
  const save = vi.fn();
  const { rerender } = render(<ShellScriptEditor title="Script" description="Bash" load={async () => "original"} save={save} />);
  const script = screen.getByLabelText("Script");
  await waitFor(() => expect(script).toBeEnabled());
  fireEvent.change(script, { target: { value: "draft" } });
  let resolveLoad!: (script: string) => void;
  rerender(<ShellScriptEditor title="Script" description="Bash" load={() => new Promise((resolve) => { resolveLoad = resolve; })} save={save} />);
  expect(screen.getByRole("button", { name: "Discard changes" })).toBeDisabled();
  await act(async () => { resolveLoad("next script"); });
  expect(script).toHaveValue("next script");
});
