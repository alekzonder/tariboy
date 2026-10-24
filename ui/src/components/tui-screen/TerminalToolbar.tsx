import { HelpCircle, Keyboard } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SendFilesButton } from "@/components/SendFilesButton";
import { HOTKEY_BYTES } from "@/lib/terminalBytes";
import { cn } from "@/lib/utils";
import type { TmuxScrollback } from "./useTmuxScrollback";

// One-click hotkeys, each a raw byte sequence written straight to the socket.
const HOTKEYS: { label: string; bytes: string }[] = [
  { label: "Esc", bytes: HOTKEY_BYTES.esc },
  { label: "Enter", bytes: HOTKEY_BYTES.enter },
  { label: "Ctrl-C", bytes: HOTKEY_BYTES.ctrlc },
  { label: "↑", bytes: HOTKEY_BYTES.up },
  { label: "↓", bytes: HOTKEY_BYTES.down },
];

const SCROLLBACK_HINT =
  "Live input is paused while you browse history — press q in the terminal or " +
  "use Exit scrollback to return to live output.";

/** Controls under the live terminal: Send files, Compose, hotkeys, the tmux
 * scrollback buttons with their live region, and Help. */
export function TerminalToolbar({
  workspaceSurface,
  closed,
  send,
  scrollback,
  scrollbackButtonRef,
  onUploaded,
  onCompose,
  onHelp,
}: {
  workspaceSurface: boolean;
  closed: boolean;
  send: (bytes: string) => void;
  scrollback: Omit<TmuxScrollback, "scrollbackButtonRef">;
  scrollbackButtonRef: TmuxScrollback["scrollbackButtonRef"];
  onUploaded: (paths: string[]) => void;
  onCompose: () => void;
  onHelp: () => void;
}) {
  const { copyModeRequested, exitAnnouncement } = scrollback;
  return (
    <div
      data-testid="terminal-toolbar"
      className={cn(
        "flex shrink-0 flex-wrap items-center gap-2",
        workspaceSurface && "border-t bg-background p-2",
      )}
    >
      <SendFilesButton onUploaded={onUploaded} />
      <Button type="button" variant="outline" size="sm" onClick={onCompose}>
        <Keyboard className="size-4" />
        Compose
      </Button>
      {HOTKEYS.map((h) => (
        <Button
          key={h.label}
          type="button"
          variant="outline"
          size="sm"
          onClick={() => send(h.bytes)}
        >
          {h.label}
        </Button>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={closed}
        ref={scrollbackButtonRef}
        onClick={scrollback.enterScrollback}
      >
        Scrollback
      </Button>
      {copyModeRequested && (
        <>
          <Button type="button" variant="outline" size="sm" onClick={scrollback.pageBack}>
            Page back
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={scrollback.pageForward}>
            Page forward
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={scrollback.exitScrollback}>
            Exit scrollback
          </Button>
        </>
      )}
      {/* Persistent: a live region that unmounts announces nothing, so both
          entering and leaving scrollback are content changes inside this one
          element. Outside scrollback it is sr-only, which takes it out of the
          flex flow entirely — the live-mode toolbar gains no visible chrome,
          and the announcement is spoken without being seen. */}
      <div
        role="status"
        aria-live="polite"
        className={
          copyModeRequested ? "flex min-w-0 flex-col text-xs text-muted-foreground" : "sr-only"
        }
      >
        {copyModeRequested && (
          <>
            <span className="font-medium text-foreground">Viewing scrollback</span>
            <span>{SCROLLBACK_HINT}</span>
          </>
        )}
        {exitAnnouncement && <span className="sr-only">{exitAnnouncement}</span>}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        aria-label="help"
        title="Help"
        onClick={onHelp}
      >
        <HelpCircle className="size-4" />
      </Button>
    </div>
  );
}
