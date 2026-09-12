import { Button, Label, SendFilesButton, Textarea } from "tariboy-ui";

// Cross-host target: /terminals and the task views pass an explicit daemon so
// the upload lands on that host's shared files directory.
const BUILD_01 = {
  id: "d_build01",
  label: "build-01",
  baseURL: "https://10.0.4.11:7777",
  kind: "ssh" as const,
  token: "",
};

const Panel = ({ children, width = 620 }: { children: React.ReactNode; width?: number }) => (
  <div style={{ width, display: "grid", gap: 10 }}>{children}</div>
);

export const TerminalToolbar = () => (
  <Panel>
    <div
      style={{
        height: 170,
        borderRadius: 8,
        border: "1px solid var(--border)",
        background: "var(--color-zinc-950, #09090b)",
        color: "#d4d4d8",
        padding: 10,
        fontFamily: "var(--font-mono)",
        fontSize: 12,
        lineHeight: 1.6,
      }}
    >
      <div>builder@build-01:/work/tariboy$ git status --short</div>
      <div> M ui/src/components/SendFilesButton.tsx</div>
      <div>builder@build-01:/work/tariboy$ </div>
    </div>
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
      <SendFilesButton daemon={BUILD_01} onUploaded={() => {}} />
      <Button size="sm" variant="outline">
        Paste
      </Button>
      <span
        style={{ marginLeft: "auto", fontSize: 12, color: "var(--muted-foreground)" }}
      >
        uploads land in /work/files on build-01
      </span>
    </div>
  </Panel>
);

export const NoLiveSession = () => (
  <Panel width={520}>
    <div
      style={{
        display: "grid",
        gap: 6,
        padding: "16px 14px",
        borderRadius: 10,
        border: "1px solid var(--border)",
        background: "var(--card)",
      }}
    >
      <div style={{ fontSize: 14 }}>Session not running or not interactive</div>
      <div style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
        Start the agent and enable interactive mode (Settings → Runtime config).
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 8 }}>
        <Button size="sm">Start</Button>
        <SendFilesButton daemon={BUILD_01} onUploaded={() => {}} />
      </div>
    </div>
  </Panel>
);

export const InTaskComment = () => (
  <Panel width={560}>
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
      <Label htmlFor="task-comment">Comment on TB-142</Label>
      <SendFilesButton onUploaded={() => {}} />
    </div>
    <Textarea
      id="task-comment"
      placeholder="Comment"
      defaultValue={"Repro attached.\n/work/files/TB-142-trace.log"}
      style={{ height: 92 }}
    />
  </Panel>
);

export const Disabled = () => (
  <Panel width={560}>
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
      <Label htmlFor="task-comment-busy">Comment on TB-142</Label>
      <SendFilesButton disabled onUploaded={() => {}} onUploadingChange={() => {}} />
    </div>
    <Textarea
      id="task-comment-busy"
      defaultValue={"Repro attached.\n/work/files/TB-142-trace.log"}
      disabled
      style={{ height: 92 }}
    />
    <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
      Saving the comment — the button is disabled, the same visual it shows while
      an upload of its own is in flight.
    </span>
  </Panel>
);
