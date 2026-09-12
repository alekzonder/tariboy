import { TuiScreen } from "tariboy-ui";

// TuiScreen owns a real xterm.js terminal; the websocket lives OUTSIDE it, in
// the injected `controller` (status/absent/send/sendResize/attachTerm). That
// seam is the whole point of the component's shape, so a preview supplies a
// controller whose `attachTerm` writes a captured session into the terminal
// instead of a socket doing it. The terminal, its fit addon, the toolbar, the
// hotkeys and the compose/scrollback/help affordances are all the shipped ones.

const SESSION = [
  "\x1b[38;5;244m$\x1b[0m tariboy agent attach builder",
  "\x1b[38;5;39m[builder]\x1b[0m iteration 48   goal \x1b[1mTB-142\x1b[0m   image \x1b[38;5;244mworker:v2\x1b[0m",
  "",
  "\x1b[38;5;244m>\x1b[0m ship the desktop updater contract",
  "",
  "  \x1b[38;5;244mbash\x1b[0m  cargo test -p tariboy-desktop updater",
  "        Compiling tariboy-desktop v0.58.1 (/home/agent/github/tariboy/desktop)",
  "        running 9 tests",
  "        test updater::acknowledged_before_resolve ... \x1b[32mok\x1b[0m",
  "        test updater::rejects_unsigned_manifest ... \x1b[32mok\x1b[0m",
  "  \x1b[32mok\x1b[0m    9 passed; 0 failed; finished in 8.14s",
  "",
  "  \x1b[38;5;244medit\x1b[0m  desktop/src/updater.rs  \x1b[32m+18\x1b[0m \x1b[31m-4\x1b[0m",
  "  \x1b[38;5;244medit\x1b[0m  ui/src/components/DesktopUpdates.tsx  \x1b[32m+6\x1b[0m \x1b[31m-2\x1b[0m",
  "",
  "  \x1b[33mwarn\x1b[0m  build-01 is disconnected; packager run deferred",
  "",
  "\x1b[38;5;244magent@builder\x1b[0m:\x1b[38;5;39m~/github/tariboy\x1b[0m$ ",
].join("\r\n");

interface TermLike { write: (data: string) => void }

const liveController = {
  status: "open" as const,
  absent: false,
  name: "builder",
  send: () => {},
  sendResize: () => {},
  attachTerm: (term: TermLike) => term.write(SESSION),
  reconnect: () => {},
};

const workspaceController = {
  ...liveController,
  name: "reviewer",
  attachTerm: (term: TermLike) =>
    term.write(
      [
        "\x1b[38;5;39m[reviewer]\x1b[0m iteration 12   goal \x1b[1mTB-142\x1b[0m",
        "",
        "  \x1b[38;5;244mread\x1b[0m  desktop/src/updater.rs",
        "  \x1b[31mblock\x1b[0m the shim must resolve AFTER acknowledgement, not before",
        "",
        "\x1b[38;5;244magent@reviewer\x1b[0m:\x1b[38;5;39m~/github/tariboy\x1b[0m$ ",
      ].join("\r\n"),
    ),
};

// The daemon reports no interactive session for this agent.
const absentController = {
  ...liveController,
  name: "docs-bot",
  absent: true,
  status: "closed" as const,
  attachTerm: () => {},
};

const Pane = ({ children }: { children: React.ReactNode }) => (
  <div style={{ height: 420, display: "flex", flexDirection: "column" }}>{children}</div>
);

// The agent Console tab: live terminal above, toolbar below.
export const LiveSession = () => (
  <Pane>
    <TuiScreen controller={liveController} fill daemon={null} onStart={() => {}} />
  </Pane>
);

// A workspace tile embeds the same screen flush: no rounded border, no focus
// ring, and compose text is never persisted.
export const WorkspaceTile = () => (
  <Pane>
    <TuiScreen
      controller={workspaceController}
      fill
      daemon={null}
      surface="workspace"
      persistDraft={false}
      onStart={() => {}}
    />
  </Pane>
);

// No session: the start panel, which still accepts dropped files so paths can
// be staged before the agent runs.
export const SessionNotRunning = () => (
  <Pane>
    <TuiScreen controller={absentController} fill daemon={null} onStart={() => {}} />
  </Pane>
);
