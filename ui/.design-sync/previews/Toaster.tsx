import { useEffect } from "react";
import * as DS from "tariboy-ui";
import { toast as packagedToast } from "sonner";
import { Toaster } from "tariboy-ui";

// The shipped <Toaster /> closes over the sonner copy bundled INTO
// tariboy-ui, while `import { toast } from "sonner"` here resolves to a
// second copy in node_modules — two module singletons, so toasts queued on
// the second one never reach the shipped Toaster. Prefer a `toast` re-exported
// from the bundle when one is available (see .design-sync/learnings/structure.md).
const toast = ((DS as unknown as { toast?: typeof packagedToast }).toast ??
  packagedToast);

const KEEP = { duration: Number.POSITIVE_INFINITY } as const;

// sonner's <Toaster /> subscribes to the toast queue on mount and does NOT
// replay toasts queued before that. Sibling effects run in JSX order, so the
// emitter must render AFTER <Toaster /> *and* defer to a macrotask — otherwise
// the toasts are dropped and the container never mounts (it renders null while
// the queue is empty).
function Emit({ run }: { run: () => void }) {
  useEffect(() => {
    const id = setTimeout(run, 0);
    return () => {
      clearTimeout(id);
      toast.dismiss();
    };
  }, [run]);
  return null;
}

const frame = {
  height: 240,
  width: 460,
  display: "flex",
  alignItems: "flex-end",
  justifyContent: "center",
  paddingBottom: 8,
  fontSize: 13,
  color: "var(--muted-foreground)",
} as const;

export const BuildFeedback = () => (
  <div style={frame}>
    <Toaster position="top-center" expand visibleToasts={5} />
    <Emit
      run={() => {
        toast.success("built worker:v2", KEEP);
        toast.info("image worker:v1 saved to file worker-v1.tar", KEEP);
      }}
    />
    Image actions on This daemon (local)
  </div>
);

export const Statuses = () => (
  <div style={frame}>
    <Toaster position="top-center" expand visibleToasts={6} />
    <Emit
      run={() => {
        toast.success("image worker:v2 removed", KEEP);
        toast.error("build failed: Tariboyfile.yaml not found", KEEP);
        toast.warning("Message queue full: 64 / 64", KEEP);
        toast.loading("Uploading team archive…", KEEP);
      }}
    />
    Agent workspace notifications
  </div>
);

export const WithAction = () => (
  <div style={frame}>
    <Toaster position="top-center" expand visibleToasts={5} />
    <Emit
      run={() =>
        toast.error("remove failed: image worker:v2 is in use by builder", {
          ...KEEP,
          action: { label: "Retry", onClick: () => {} },
        })
      }
    />
    Failed image removal
  </div>
);
