import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
  Badge,
  Button,
} from "tariboy-ui";
import { Trash2 } from "lucide-react";

// The page behind the overlay: the agent console header row (AgentConsoleTab)
// and the images table row (BuiltImages) are where AlertDialog is actually
// raised from, so the dimmed backdrop reads as a real screen.
const AgentToolbar = ({ children }: { children?: React.ReactNode }) => (
  <div style={{ display: "grid", gap: 12, width: 600 }}>
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <span style={{ fontSize: 14, fontWeight: 500 }}>builder</span>
      <Badge variant="secondary">running</Badge>
      <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
        worker:v2 · This daemon (local)
      </span>
      <span style={{ marginLeft: "auto", display: "flex", gap: 8 }}>{children}</span>
    </div>
    <div
      style={{
        height: 150,
        borderRadius: 8,
        border: "1px solid var(--border)",
        background: "var(--muted)",
      }}
    />
  </div>
);

const ImagesTable = ({ children }: { children?: React.ReactNode }) => (
  <div style={{ width: 600, display: "grid", gap: 8 }}>
    <div style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
      Built images · This daemon (local)
    </div>
    {["worker:v2", "worker:v1", "bare:latest"].map((ref, i) => (
      <div
        key={ref}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "8px 10px",
          fontSize: 13,
          borderRadius: 8,
          border: "1px solid var(--border)",
        }}
      >
        <span style={{ fontFamily: "var(--font-mono)" }}>{ref}</span>
        <span style={{ marginLeft: "auto", display: "flex", gap: 8 }}>
          {i === 0 ? children : null}
        </span>
      </div>
    ))}
  </div>
);

export const DeleteAgent = () => (
  <AgentToolbar>
    <AlertDialog defaultOpen>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="destructive">
          Delete
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete agent builder?</AlertDialogTitle>
          <AlertDialogDescription>
            This permanently deletes the agent and all of its durable data. This
            action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive">Delete agent</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </AgentToolbar>
);

export const WithMedia = () => (
  <ImagesTable>
    <AlertDialog defaultOpen>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="destructive" aria-label="Remove worker:v2">
          Remove
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Trash2 />
          </AlertDialogMedia>
          <AlertDialogTitle>Remove image worker:v2?</AlertDialogTitle>
          <AlertDialogDescription>
            Deletes this immutable runnable image. Original build files are not
            managed by Tariboy.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive">Remove image</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </ImagesTable>
);

export const SizeSm = () => (
  <AgentToolbar>
    <AlertDialog defaultOpen>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="outline">
          Stop
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>Stop builder?</AlertDialogTitle>
          <AlertDialogDescription>
            The session ends and the autopilot loop pauses. Durable data is kept.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction>Stop agent</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </AgentToolbar>
);

export const Pending = () => (
  <AgentToolbar>
    <AlertDialog defaultOpen>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="destructive">
          Delete
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete agent docs-bot?</AlertDialogTitle>
          <AlertDialogDescription>
            This permanently deletes the agent and all of its durable data. This
            action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled>
            Deleting…
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </AgentToolbar>
);
