import { AttachButton, Button, Label, Textarea } from "tariboy-ui";

// AttachButton picks a file, uploads it to the daemon's shared directory and
// hands the saved absolute host path back so the caller can splice it into the
// issue description. The hidden <input type="file"> means the button is the
// whole visible surface, so every cell composes it in a real footer row.
// `daemon` follows Daemon in src/lib/daemons.ts (omit it for the local daemon).

const remote = {
  id: "build-01",
  label: "build-01",
  baseURL: "https://10.0.4.11:7777",
  token: "tb_live_9c1f",
  kind: "ssh" as const,
};

export const InIssueComposer = () => (
  <div style={{ width: 520, display: "flex", flexDirection: "column", gap: 8 }}>
    <Label htmlFor="issue-body">Description</Label>
    <Textarea
      id="issue-body"
      rows={4}
      defaultValue={
        "TB-142 — the desktop updater must resolve after acknowledgement.\n\nRepro log attached: /srv/tariboy/shared/updater-run.log"
      }
    />
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <AttachButton onAttached={() => {}} />
      <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--muted-foreground)" }}>
        uploads to the shared dir
      </span>
      <span style={{ marginLeft: "auto" }}>
        <Button size="sm">Create issue</Button>
      </span>
    </div>
  </div>
);

export const RemoteDaemon = () => (
  <div style={{ width: 380, display: "flex", alignItems: "center", gap: 8 }}>
    <AttachButton onAttached={() => {}} daemon={remote} />
    <span style={{ fontSize: 13, color: "var(--muted-foreground)" }}>
      Host: <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace" }}>build-01</span>
    </span>
  </div>
);

export const NoDaemonSelected = () => (
  <div style={{ width: 380, display: "flex", alignItems: "center", gap: 8 }}>
    <AttachButton onAttached={() => {}} daemon={null} />
    <span style={{ fontSize: 13, color: "var(--muted-foreground)" }}>This daemon (local)</span>
  </div>
);
