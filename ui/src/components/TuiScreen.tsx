import { useState, type ClipboardEvent as ReactClipboardEvent } from "react";
import { toast } from "sonner";
import { useFileDropTarget } from "@/hooks/useFileDropTarget";
import { useSendFiles } from "@/hooks/useSendFiles";
import type { TerminalSocketController } from "@/hooks/useTerminalSocket";
import { buildPathsText, useTuiDraft } from "@/lib/tui";
import { cn } from "@/lib/utils";
import { ComposeDialog } from "./tui-screen/ComposeDialog";
import { TerminalHelpDialog } from "./tui-screen/TerminalHelpDialog";
import { TerminalToolbar } from "./tui-screen/TerminalToolbar";
import { TuiAbsentPanel } from "./tui-screen/TuiAbsentPanel";
import { useTmuxScrollback } from "./tui-screen/useTmuxScrollback";
import { useXtermTerminal } from "./tui-screen/useXtermTerminal";

/** The interactive terminal screen. Mounts a live xterm.js terminal driven by
 * the streaming websocket {@link TerminalSocketController}: PTY bytes are
 * written into the terminal and keystrokes flow back over `controller.send`.
 * A toolbar offers hotkeys, a compose modal (optionally persisted per agent),
 * and a "Send files" button that injects uploaded paths into the stream. When
 * the daemon reports no interactive session, a Start panel is shown instead. */
export function TuiScreen({
  controller,
  onStart,
  fill = false,
  persistDraft = true,
  surface = "standalone",
}: {
  controller: TerminalSocketController;
  onStart?: () => void;
  /** Fill the parent's height (flex column) instead of a fixed 32rem screen. */
  fill?: boolean;
  /** Retain compose text in localStorage. Workspace tiles disable this so
   * operator text and uploaded paths never enter persisted workspace state. */
  persistDraft?: boolean;
  /** Standalone Terminal tab chrome or a flush surface embedded in a Workspace pane. */
  surface?: "standalone" | "workspace";
}) {
  const { absent, name, send, status } = controller;
  const [focused, setFocused] = useState(false);

  // Compose modal: Agent mode persists its per-agent draft, while Workspace
  // keeps the draft in component memory only. On Send the text is written to
  // the stream as-is.
  const [modalOpen, setModalOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const { scrollbackButtonRef, ...scrollback } = useTmuxScrollback(controller);
  // `onTerminalInput` is a dep-free useCallback, so handing it to the mount
  // effect never recreates the terminal on a scrollback toggle.
  const containerRef = useXtermTerminal(controller, scrollback.onTerminalInput);
  const draft = useTuiDraft(name, "terminal", persistDraft);
  const workspaceSurface = surface === "workspace";
  const liveUpload = useSendFiles({
    onUploaded: (paths) => {
      draft.append(buildPathsText(paths));
      setModalOpen(true);
    },
  });
  const absentUpload = useSendFiles({
    onUploaded: (paths) => toast.success(`uploaded: ${paths.join(", ")}`),
  });
  const liveDrop = useFileDropTarget(liveUpload.sendFiles);
  const absentDrop = useFileDropTarget(absentUpload.sendFiles);

  // Paste into the (focused, empty) terminal wrapper composes instead of
  // dropping a raw multi-line paste straight onto the shell.
  const onPaste = (e: ReactClipboardEvent) => {
    const text = e.clipboardData.getData("text");
    if (!text) return;
    e.preventDefault();
    draft.append(text);
    setModalOpen(true);
  };

  if (absent) {
    return (
      <TuiAbsentPanel
        drop={absentDrop}
        onStart={onStart}
        fill={fill}
        workspaceSurface={workspaceSurface}
      />
    );
  }

  return (
    // In fill mode this is a flex child of the pane's column: `min-h-0 flex-1`
    // makes it take exactly the space left by the pane header (and shrink with
    // the window) instead of claiming 100% of the pane and overflowing it.
    <div
      className={cn(
        fill ? "flex min-h-0 flex-1 flex-col" : workspaceSurface ? "flex flex-col" : "space-y-2",
        fill && !workspaceSurface && "gap-2",
      )}
    >
      <div
        ref={containerRef}
        data-testid="tui-screen"
        data-terminal-surface={surface}
        data-file-drag-active={liveDrop.dragActive}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onPaste={onPaste}
        onDragOver={liveDrop.onDragOver}
        onDragLeave={liveDrop.onDragLeave}
        onDrop={liveDrop.onDrop}
        className={cn(
          "overflow-hidden bg-zinc-950 p-2",
          workspaceSurface ? "border-0" : "rounded-md border",
          fill ? "min-h-0 flex-1" : "h-[32rem]",
          // Mirror the old capture affordance: ring the terminal when focused.
          focused && !workspaceSurface && "ring-2 ring-primary",
          liveDrop.dragActive && "ring-2 ring-primary",
        )}
      />

      <TerminalToolbar
        workspaceSurface={workspaceSurface}
        closed={status === "closed"}
        send={send}
        scrollback={scrollback}
        scrollbackButtonRef={scrollbackButtonRef}
        onUploaded={(paths) => {
          draft.append(buildPathsText(paths));
          setModalOpen(true);
        }}
        onCompose={() => setModalOpen(true)}
        onHelp={() => setHelpOpen(true)}
      />

      <TerminalHelpDialog open={helpOpen} onOpenChange={setHelpOpen} />

      <ComposeDialog open={modalOpen} onOpenChange={setModalOpen} draft={draft} send={send} />
    </div>
  );
}
