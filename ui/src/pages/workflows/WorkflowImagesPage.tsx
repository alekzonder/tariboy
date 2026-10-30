import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus } from "lucide-react";
import type { WorkflowImageListing } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { errorText } from "@/lib/errors";
import WorkflowBuildFromDirectory from "./WorkflowBuildFromDirectory";
import { boundTo, groupWorkflowImages, useQueueBindings } from "./workflowImages";

const CELL = "px-3 py-2";

/** The workflow images of one host: one row per name, its versions in the detail. */
export default function WorkflowImagesPage({ basePath }: { basePath: string }) {
  const api = useApi();
  const [images, setImages] = useState<WorkflowImageListing[] | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [adding, setAdding] = useState(false);
  const bindings = useQueueBindings();
  const reloadBindings = bindings.reload;

  useEffect(() => {
    if (!api.ready) return;
    let alive = true;
    api.workflows.listWorkflowImages().then(
      (result) => {
        if (alive) {
          setImages(result);
          setError("");
        }
      },
      (err) => {
        if (alive) setError(errorText(err));
      },
    );
    return () => {
      alive = false;
    };
  }, [api, revision]);
  const reload = useCallback(() => {
    setRevision((value) => value + 1);
    reloadBindings();
  }, [reloadBindings]);
  const groups = useMemo(() => groupWorkflowImages(images ?? []), [images]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div data-toolbar className="flex shrink-0 flex-nowrap items-center gap-3 px-4 py-2">
        <Button size="xs" className="ml-auto shrink-0" onClick={() => setAdding(true)}>
          <Plus aria-hidden="true" />
          Add workflow
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto px-4 pb-4">
        {error && (
          <div role="alert" className="flex items-center gap-2 text-sm text-destructive">
            <span>{error}</span>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                setError("");
                reload();
              }}
            >
              Retry
            </Button>
          </div>
        )}
        {images === null ? (
          !error && <p className="text-sm text-muted-foreground">Loading workflows…</p>
        ) : images.length === 0 ? (
          <p className="text-sm text-muted-foreground">No workflows on this host.</p>
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted-foreground">
              <tr>
                {["Name", "Version", "Bound to"].map((label) => (
                  <th key={label} className={`${CELL} font-medium`}>
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {groups.map(({ name, tags }) => {
                const newest = tags[0];
                const bound = bindings.loaded
                  ? boundTo(
                      bindings,
                      tags.map((image) => image.digest),
                    )
                  : { text: "…" };
                return (
                  <tr key={name} className="border-t">
                    <td className={CELL}>
                      <Link
                        className="font-medium text-primary hover:underline"
                        to={`${basePath}/${encodeURIComponent(name)}/${encodeURIComponent(newest.tag)}`}
                      >
                        {name}
                      </Link>
                    </td>
                    <td className={CELL}>{newest.version}</td>
                    <td className={`${CELL} text-xs`}>
                      <span title={bound.title}>{bound.text}</span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-3xl">
          <DialogTitle>Add workflow</DialogTitle>
          <WorkflowBuildFromDirectory onBuilt={reload} />
        </DialogContent>
      </Dialog>
    </div>
  );
}
