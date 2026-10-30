import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ApiError, type WorkflowImageManifest } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { useWorkflowConfirm } from "@/pages/tasks/WorkflowConfirm";
import { errorText } from "@/lib/errors";
import { useMounted } from "@/pages/tasks/workflowShared";
import { WorkflowTransferDialog } from "./WorkflowTransferDialog";
import { WorkflowVersions } from "./WorkflowVersions";
import { boundTo, useQueueBindings, useTransferDaemons } from "./workflowImages";
import {
  ArtifactsList,
  EnvList,
  FilesTable,
  LimitsTable,
  SecretsList,
  StatusesTable,
  WorkflowImageHeader,
} from "./workflowImageSections";

/** One workflow image, read from its manifest. Every manifest value renders as plain text. */
export default function WorkflowImageDetail({
  name,
  tag,
  basePath,
}: {
  name: string;
  tag: string;
  basePath: string;
}) {
  const api = useApi();
  const [manifest, setManifest] = useState<WorkflowImageManifest | null>(null);
  const [error, setError] = useState("");
  const [removeError, setRemoveError] = useState("");
  const [busy, setBusy] = useState(false);
  const [transfer, setTransfer] = useState(false);
  const bindings = useQueueBindings();
  const transferDaemons = useTransferDaemons();
  const { confirm, dialog } = useWorkflowConfirm();
  const navigate = useNavigate();
  const mounted = useMounted();

  useEffect(() => {
    if (!api.ready) return;
    let alive = true;
    api.workflows.getWorkflowImage(name, tag).then(
      (result) => {
        if (alive) {
          setManifest(result);
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
  }, [name, tag, api]);

  if (error && !manifest)
    return (
      <p role="alert" className="p-3 text-sm text-destructive">
        {error}
      </p>
    );
  if (!manifest)
    return <p className="p-3 text-sm text-muted-foreground">Loading workflow image…</p>;

  const ref = `${name}:${tag}`;
  const definition = manifest.definition;
  const bound = bindings.loaded ? boundTo(bindings, manifest.digest) : { text: "…" };
  // A digest names content, not a tag: there is no tag to remove.
  const byDigest = tag === manifest.digest;

  const remove = () =>
    confirm({
      title: `Remove tag ${ref}?`,
      description: "When no other tag names this content, the content is deleted too.",
      action: `Remove ${ref}`,
      run: () => {
        setRemoveError("");
        setBusy(true);
        api.workflows.removeWorkflowImage(name, tag).then(
          () => navigate(basePath),
          (err) => {
            if (!mounted.current) return;
            setBusy(false);
            setRemoveError(
              err instanceof ApiError && err.code === "workflow_in_use"
                ? `${err.message}. Bound to: ${bound.text}`
                : errorText(err),
            );
          },
        );
      },
    });

  return (
    <div className="h-full space-y-5 overflow-auto p-3 text-sm">
      <nav className="text-xs text-muted-foreground">
        <Link className="text-primary hover:underline" to={basePath}>
          Workflows
        </Link>{" "}
        / <span className="font-mono">{ref}</span>
      </nav>
      <WorkflowImageHeader
        manifest={manifest}
        bound={bound}
        uploadDisabled={!transferDaemons.ready}
        onUpload={() => setTransfer(true)}
        canRemove={!byDigest}
        busy={busy}
        onRemove={remove}
        error={error}
        removeError={removeError}
      />
      <StatusesTable definition={definition} />
      <LimitsTable definition={definition} />
      <ArtifactsList definition={definition} />
      <SecretsList definition={definition} />
      <EnvList definition={definition} />
      <FilesTable files={manifest.files} />
      <WorkflowVersions
        name={name}
        tag={tag}
        basePath={basePath}
        bindings={bindings}
        transferDaemons={transferDaemons}
      />

      {dialog}
      {transfer && (
        <WorkflowTransferDialog
          open
          onOpenChange={setTransfer}
          name={name}
          tag={tag}
          daemons={transferDaemons.daemons}
          onComplete={() => undefined}
        />
      )}
    </div>
  );
}
