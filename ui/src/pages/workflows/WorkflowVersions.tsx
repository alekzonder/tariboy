import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import type { WorkflowImageListing } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { errorText } from "@/lib/errors";
import { relativeTime } from "@/lib/time";
import { WorkflowTransferDialog } from "./WorkflowTransferDialog";
import {
  boundTo,
  groupWorkflowImages,
  type QueueBindings,
  type useTransferDaemons,
} from "./workflowImages";

const CELL = "px-3 py-2";

/** The tags of one workflow name, each with its bindings and an upload to
 *  other servers. `tag` is the one the page shows. */
export function WorkflowVersions({
  name,
  tag,
  basePath,
  bindings,
  transferDaemons,
}: {
  name: string;
  tag: string;
  basePath: string;
  bindings: Omit<QueueBindings, "reload">;
  transferDaemons: ReturnType<typeof useTransferDaemons>;
}) {
  const api = useApi();
  const [tags, setTags] = useState<WorkflowImageListing[] | null>(null);
  const [error, setError] = useState("");
  const [transfer, setTransfer] = useState<string | null>(null);

  useEffect(() => {
    if (!api.ready) return;
    let alive = true;
    api.workflows.listWorkflowImages().then(
      (result) => {
        if (alive) setTags(groupWorkflowImages(result).find((g) => g.name === name)?.tags ?? []);
      },
      (err) => {
        if (alive) setError(errorText(err));
      },
    );
    return () => {
      alive = false;
    };
  }, [api, name]);

  return (
    <section aria-label="Versions" className="space-y-2">
      <h2 className="font-medium">Versions</h2>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      {tags && (
        <table className="w-full text-sm">
          <thead className="text-left text-xs text-muted-foreground">
            <tr>
              {["Tag", "Version", "Built", "Bound to", ""].map((label) => (
                <th key={label} className={`${CELL} font-medium`}>
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {tags.map((image) => {
              const ref = `${image.name}:${image.tag}`;
              const bound = bindings.loaded ? boundTo(bindings, image.digest) : { text: "…" };
              return (
                <tr
                  key={ref}
                  aria-current={image.tag === tag ? "true" : undefined}
                  className={image.tag === tag ? "border-t bg-muted/40" : "border-t"}
                >
                  <td className={CELL}>
                    <Link
                      className="font-mono text-primary hover:underline"
                      to={`${basePath}/${encodeURIComponent(image.name)}/${encodeURIComponent(image.tag)}`}
                    >
                      {image.tag}
                    </Link>
                  </td>
                  <td className={CELL}>{image.version}</td>
                  <td className={`${CELL} text-xs text-muted-foreground`} title={image.built_at}>
                    {relativeTime(image.built_at)}
                  </td>
                  <td className={`${CELL} text-xs`}>
                    <span title={bound.title}>{bound.text}</span>
                  </td>
                  <td className={`${CELL} text-right`}>
                    <Button
                      size="xs"
                      variant="outline"
                      aria-label={`Upload to servers ${ref}`}
                      disabled={!transferDaemons.ready}
                      onClick={() => setTransfer(image.tag)}
                    >
                      Upload to servers
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      {transfer !== null && (
        <WorkflowTransferDialog
          open
          onOpenChange={(open) => {
            if (!open) setTransfer(null);
          }}
          name={name}
          tag={transfer}
          daemons={transferDaemons.daemons}
          onComplete={() => undefined}
        />
      )}
    </section>
  );
}
