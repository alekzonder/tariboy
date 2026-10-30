import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { TerminalHelpDialog } from "./TerminalHelpDialog";
import { TerminalToolbar } from "./TerminalToolbar";

vi.mock("@/components/SendFilesButton", () => ({ SendFilesButton: () => null }));

afterEach(() => vi.unstubAllEnvs());

function renderToolbar(send = vi.fn()) {
  render(
    <TerminalToolbar
      workspaceSurface
      closed={false}
      send={send}
      scrollback={{
        copyModeRequested: false,
        exitAnnouncement: "",
        enterScrollback: vi.fn(),
        pageBack: vi.fn(),
        pageForward: vi.fn(),
        exitScrollback: vi.fn(),
        onTerminalInput: vi.fn(),
      }}
      scrollbackButtonRef={{ current: null }}
      onUploaded={vi.fn()}
      onCompose={vi.fn()}
      onHelp={vi.fn()}
    />,
  );
  return send;
}

describe("TerminalToolbar keys", () => {
  it("keeps the hotkeys behind Keys on desktop", () => {
    const send = renderToolbar();
    const keys = screen.getByRole("button", { name: "Keys" });
    expect(keys).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("button", { name: "Esc" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "help" })).toBeInTheDocument();

    fireEvent.click(keys);
    expect(keys).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(screen.getByRole("button", { name: "Esc" }));
    expect(send).toHaveBeenCalledOnce();
  });

  it("shows the hotkeys at once on Android", () => {
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    renderToolbar();
    expect(screen.getByRole("button", { name: "Keys" })).toHaveAttribute("aria-expanded", "true");
    for (const name of ["Esc", "Enter", "Ctrl-C", "↑", "↓"]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
  });
});

it("starts the help with what the terminal is for", () => {
  render(<TerminalHelpDialog open onOpenChange={vi.fn()} />);
  expect(screen.getByRole("dialog")).toHaveTextContent(
    /^Terminal helpTerminal is for working with the agent by hand\. For work it should carry out on its own, give it a task in Work; it runs it when it is on\./,
  );
});
