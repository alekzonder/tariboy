import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  Textarea,
} from "tariboy-ui";

// What sits under the overlay: the terminals workspace the dialogs are raised
// from. Keeps the dimmed/blurred backdrop honest instead of dimming blank white.
const Workspace = () => (
  <div style={{ display: "flex", gap: 16, width: 720 }}>
    <div style={{ width: 180, display: "grid", gap: 6, alignContent: "start" }}>
      <div style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
        This daemon (local)
      </div>
      {["builder", "reviewer", "packager", "docs-bot"].map((name) => (
        <div key={name} style={{ fontSize: 13, padding: "4px 8px", borderRadius: 6 }}>
          {name}
        </div>
      ))}
    </div>
    <div
      style={{
        flex: 1,
        height: 220,
        borderRadius: 8,
        border: "1px solid var(--border)",
        background: "var(--muted)",
      }}
    />
  </div>
);

const field: React.CSSProperties = { display: "grid", gap: 6 };

export const NewAgent = () => (
  <>
    <Workspace />
    <Dialog defaultOpen>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>New agent</DialogTitle>
          <DialogDescription>
            Choose a target and image, then configure identity, runtime, Autopilot,
            and lifecycle settings.
          </DialogDescription>
        </DialogHeader>
        <div style={{ display: "grid", gap: 16 }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <div style={field}>
              <Label htmlFor="new-agent-host">Host</Label>
              <Select defaultValue="local">
                <SelectTrigger id="new-agent-host">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="local">This daemon (local)</SelectItem>
                  <SelectItem value="build-01">build-01</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div style={field}>
              <Label htmlFor="new-agent-image">Image</Label>
              <Select defaultValue="worker:v2">
                <SelectTrigger id="new-agent-image">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="worker:v2">worker:v2</SelectItem>
                  <SelectItem value="worker:v1">worker:v1</SelectItem>
                  <SelectItem value="bare:latest">bare:latest</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div style={field}>
            <Label htmlFor="new-agent-name">Name</Label>
            <Input id="new-agent-name" defaultValue="packager" />
          </div>
          <div style={field}>
            <Label htmlFor="new-agent-goal">Goal</Label>
            <Textarea
              id="new-agent-goal"
              rows={3}
              defaultValue="Package each merged change for release and keep the changelog current."
            />
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <Switch id="new-agent-autopilot" defaultChecked />
            <Label htmlFor="new-agent-autopilot">Start Autopilot after create</Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline">Cancel</Button>
          <Button>Create agent</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>
);

export const Confirm = () => (
  <>
    <Workspace />
    <Dialog defaultOpen>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Transfer worker:v2</DialogTitle>
          <DialogDescription>
            Export the image from This daemon (local) and apply it on the selected
            hosts.
          </DialogDescription>
        </DialogHeader>
        <div style={{ display: "grid", gap: 8 }}>
          {[
            { host: "build-01", note: "ready" },
            { host: "build-02", note: "already has worker:v2" },
          ].map((row) => (
            <div
              key={row.host}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                fontSize: 13,
                padding: "6px 8px",
                borderRadius: 6,
                border: "1px solid var(--border)",
              }}
            >
              <span>{row.host}</span>
              <span style={{ marginLeft: "auto", color: "var(--muted-foreground)", fontSize: 12 }}>
                {row.note}
              </span>
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline">Cancel</Button>
          <Button>Start transfer</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>
);

export const FooterCloseAction = () => (
  <>
    <Workspace />
    <Dialog defaultOpen>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Host build-01 updated</DialogTitle>
          <DialogDescription>
            The daemon on build-01 is now on 0.58.1 and reconnected. No agents were
            restarted.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton />
      </DialogContent>
    </Dialog>
  </>
);

export const NoCloseButton = () => (
  <>
    <Workspace />
    <Dialog defaultOpen>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>TB-142</DialogTitle>
          <DialogDescription>
            Autopilot loop stalls when the image manifest is missing a harness.
          </DialogDescription>
        </DialogHeader>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <Badge variant="secondary">in review</Badge>
          <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
            assigned to reviewer · worker:v2
          </span>
        </div>
        <DialogFooter>
          <Button variant="outline">Reassign</Button>
          <Button>Mark done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>
);
