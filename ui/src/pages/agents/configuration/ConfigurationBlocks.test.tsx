import { expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { ConfigurationBlocks } from "./ConfigurationBlocks";
import { BLOCKS } from "./registry";
import { useConfigDraft } from "./useConfigDraft";
import type { ConfigSources } from "./useConfigSources";

it("edits message settings in the shared page draft without its own Save", () => {
  function Section() {
    const draft = useConfigDraft({
      baseline: { "messages-batch": "10", "messages-max-queue": "100" },
      units: [{ id: "messages", keys: ["messages-batch", "messages-max-queue"], submit: async () => {} }],
      reload: async () => {},
    });
    return <ConfigurationBlocks
      block={BLOCKS.find((block) => block.id === "messages")!}
      draft={draft}
      matched={new Set(["messages-batch"])}
      dirty={draft.dirtyKeys.length}
      sources={{} as ConfigSources}
      target={null}
      name="worker"
      stopped
      refresh={vi.fn()}
    />;
  }
  render(<Section />);
  const batch = screen.getByLabelText("Messages per iteration");
  fireEvent.change(batch, { target: { value: "20" } });
  expect(batch).toHaveValue(20);
  expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
});
