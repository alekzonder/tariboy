import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useApi } from "@/components/apiContext";
import type { AgentImageStatus, AgentView } from "@/lib/api";
import { useMultiHost } from "@/lib/navRoutes";
import { guard } from "@/lib/toast-guard";
import { ActionRow } from "./ConfigControls";
import { imageHref } from "./generalImageModel";
import type { ConfigSources } from "./useConfigSources";

// PendingImage shows an image assignment that waits for the next iteration,
// and why its activation failed.
export function PendingImage({ imageStatus }: { imageStatus: AgentImageStatus }) {
  const targetId = useApi().target?.id ?? "";
  const multiHost = useMultiHost();
  const { pending } = imageStatus;
  if (!pending.ref && !pending.error) return null;
  return (
    <div className="space-y-1 text-sm">
      {pending.ref && (
        <div>
          Pending:{" "}
          <Link
            className="font-mono text-primary hover:underline"
            to={imageHref(targetId, pending.ref, multiHost)}
          >
            {pending.ref}
          </Link>
        </div>
      )}
      {pending.error && (
        <p role="alert" className="text-destructive">
          {pending.error}
        </p>
      )}
    </div>
  );
}

// ImageActions copies the digest and retries or cancels a pending image
// assignment. Each action runs at once and is never part of Save.
export function ImageActions({
  view,
  imageStatus,
  sources,
  name,
  refresh,
}: {
  view: AgentView | null;
  imageStatus: AgentImageStatus | null;
  sources: ConfigSources;
  name: string;
  refresh: () => void;
}) {
  const api = useApi();
  const [copied, setCopied] = useState(false);
  const [imageBusy, setImageBusy] = useState(false);
  const pending = imageStatus?.pending;
  if (!view?.digest && !pending?.ref) return null;
  const imageAction = async (label: string, run: () => Promise<unknown>) => {
    setImageBusy(true);
    await guard(label, async () => {
      await run();
      await sources.loadImages();
      refresh();
    });
    setImageBusy(false);
  };
  const copyDigest = async (digest: string) => {
    try {
      await navigator.clipboard.writeText(digest);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };
  return (
    <ActionRow>
      {view?.digest && (
        <Button size="sm" variant="outline" onClick={() => void copyDigest(view.digest)}>
          Copy digest
        </Button>
      )}
      {copied && (
        <span role="status" className="text-xs text-muted-foreground">
          Digest copied
        </span>
      )}
      {pending?.error && (
        <Button
          size="sm"
          variant="outline"
          disabled={imageBusy}
          onClick={() =>
            void imageAction("image assignment", () => api.agents.agentImageSet(name, pending.ref))
          }
        >
          Retry
        </Button>
      )}
      {pending?.ref && (
        <Button
          size="sm"
          variant="outline"
          disabled={imageBusy}
          onClick={() =>
            void imageAction("pending image cancellation", () => api.agents.agentImageCancel(name))
          }
        >
          Cancel pending
        </Button>
      )}
    </ActionRow>
  );
}
