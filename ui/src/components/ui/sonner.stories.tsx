import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, type ReactNode } from "react";
import { toast } from "sonner";
import { Toaster } from "./sonner";

const meta = { component: Toaster } satisfies Meta<typeof Toaster>;
export default meta;

type Story = StoryObj;

const KEEP = { duration: Number.POSITIVE_INFINITY } as const;

// sonner's <Toaster /> subscribes to the toast queue on mount and does NOT
// replay toasts queued before that. Sibling effects run in JSX order, so the
// emitter must render AFTER <Toaster /> and defer to a macrotask.
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

function Frame({ visible, children }: { visible: number; children: ReactNode }) {
  return (
    <div className="flex h-60 w-115 items-end justify-center pb-2 text-body text-muted-foreground">
      <Toaster position="top-center" expand visibleToasts={visible} />
      {children}
    </div>
  );
}

export const BuildFeedback: Story = {
  render: () => (
    <Frame visible={5}>
      <Emit
        run={() => {
          toast.success("built worker:v2", KEEP);
          toast.info("image worker:v1 saved to file worker-v1.tar", KEEP);
        }}
      />
      Image actions on This daemon (local)
    </Frame>
  ),
};

export const Statuses: Story = {
  render: () => (
    <Frame visible={6}>
      <Emit
        run={() => {
          toast.success("image worker:v2 removed", KEEP);
          toast.error("build failed: Tariboyfile.yaml not found", KEEP);
          toast.warning("Message queue full: 64 / 64", KEEP);
          toast.loading("Uploading team archive…", KEEP);
        }}
      />
      Agent workspace notifications
    </Frame>
  ),
};

export const WithAction: Story = {
  render: () => (
    <Frame visible={5}>
      <Emit
        run={() =>
          toast.error("remove failed: image worker:v2 is in use by builder", {
            ...KEEP,
            action: { label: "Retry", onClick: () => {} },
          })
        }
      />
      Failed image removal
    </Frame>
  ),
};
