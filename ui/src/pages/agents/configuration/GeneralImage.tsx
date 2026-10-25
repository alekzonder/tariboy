import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useApi } from "@/components/apiContext";
import type { AgentImageStatus } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import type { AgentView } from "@/lib/types";
import { ActionRow } from "./ConfigControls";
import { imageHref, imageSides, nextReasonText, updatesOnNext } from "./generalImageModel";
import type { ConfigSources } from "./useConfigSources";

type ImageSideValue = AgentImageStatus["current"] | AgentImageStatus["next"];

function ImageSide({
  label,
  value,
  targetId,
}: {
  label: string;
  value: NonNullable<ImageSideValue>;
  targetId: string;
}) {
  return (
    <div role="group" aria-label={label} className="space-y-0.5">
      <div className="font-medium">{label}</div>
      <Link className="font-mono text-primary hover:underline" to={imageHref(targetId, value.ref)}>
        {value.ref}
      </Link>
      <div>
        {value.error ? "Version unavailable" : value.image_version || "Version not specified"}
      </div>
      <div className="font-mono text-xs break-all">{value.digest || "Digest unavailable"}</div>
      {value.error && (
        <p role="alert" className="mt-1 text-destructive">
          {value.error}
        </p>
      )}
    </div>
  );
}

function NextImageNote({ imageStatus }: { imageStatus: AgentImageStatus }) {
  const next = imageStatus.next;
  if (!next) return null;
  return (
    <div className="space-y-1 sm:col-span-2">
      <p className="text-xs text-muted-foreground">{nextReasonText(next.reason)}</p>
      {updatesOnNext(imageStatus) && <p role="status">Update on next iteration</p>}
      <p className="text-xs text-muted-foreground">
        This preview is not a reservation; the image may change before launch.
      </p>
    </div>
  );
}

// ImageIdentity shows the current and next image versions with their digests,
// why the next one was chosen, and any pending assignment.
export function ImageIdentity({
  imageStatus,
  stopped,
}: {
  imageStatus: AgentImageStatus;
  stopped: boolean;
}) {
  const targetId = useApi().target?.id ?? "";
  return (
    <div className="grid gap-4 text-sm sm:grid-cols-2">
      {imageSides(imageStatus, stopped).map(
        ({ label, value }) =>
          value && <ImageSide key={label} label={label} value={value} targetId={targetId} />,
      )}
      <NextImageNote imageStatus={imageStatus} />
      {imageStatus.pending.ref && (
        <div className="sm:col-span-2">
          Pending:{" "}
          <Link
            className="font-mono text-primary hover:underline"
            to={imageHref(targetId, imageStatus.pending.ref)}
          >
            {imageStatus.pending.ref}
          </Link>
        </div>
      )}
      {imageStatus.pending.error && (
        <p role="alert" className="text-destructive sm:col-span-2">
          {imageStatus.pending.error}
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
