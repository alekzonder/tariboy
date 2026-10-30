import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import type {
  AgentPool,
  QueueWorkflow,
  WorkflowImageDefinition,
  WorkflowImageListing,
} from "@/lib/api";
import { EMPTY, LABEL, MONO, SelectShell } from "./panelStyles";
import { useWorkflowConfirm } from "./WorkflowConfirm";
import WorkflowStatusMap from "./WorkflowStatusMap";
import { errorText } from "@/lib/errors";
import { classifyBindError, stillEmptyPools } from "./queueSettingsModel";

/** Images by name, then tag with `latest` first. */
function sortImages(images: WorkflowImageListing[]): WorkflowImageListing[] {
  const rank = (image: WorkflowImageListing) => (image.tag === "latest" ? 0 : 1);
  return [...images].sort(
    (a, b) => a.name.localeCompare(b.name) || rank(a) - rank(b) || a.tag.localeCompare(b.tag),
  );
}

/** The tag of the bound content, the one equal to its version first; the digest when no tag names it. */
function bindingRef(binding: QueueWorkflow, images: WorkflowImageListing[]): string {
  const tags = images.filter(
    (image) => image.name === binding.name && image.digest === binding.digest,
  );
  return (tags.find((image) => image.tag === binding.version) ?? tags[0])?.tag ?? binding.digest;
}

/**
 * The Workflow tab of a queue: the bound image, a select to bind another, and
 * the statuses of the bound or the chosen image with the pool that owns each.
 * A bind refused for empty pools or missing secrets lists them with a way to
 * the tab that fixes them; the pools list shrinks as `pools` gains agents.
 */
export default function QueueWorkflowSettings({
  queue,
  pools,
  onOpenTab,
  onPoolsMissing,
  onSecretsMissing,
  onChanged,
}: {
  queue: string;
  /** The queue's pools, kept current by the Pools tab. */
  pools: AgentPool[];
  onOpenTab: (tab: "pools" | "environment") => void;
  /** Told of the pools a refused bind named, and of null when a new attempt starts. */
  onPoolsMissing: (names: string[] | null) => void;
  /** Told of the secrets a refused bind named, and of null when a new attempt starts. */
  onSecretsMissing: (missing: { names: string[]; message: string } | null) => void;
  /** Told after the binding changed here. */
  onChanged: () => void;
}) {
  const api = useApi();
  const [binding, setBinding] = useState<QueueWorkflow | null>(null);
  // True only after a load succeeded: until then the binding is unknown.
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [images, setImages] = useState<WorkflowImageListing[]>([]);
  const [ref, setRef] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [emptyPools, setEmptyPools] = useState<{ names: string[]; message: string } | null>(null);
  const [missingSecrets, setMissingSecrets] = useState<{ names: string[]; message: string } | null>(
    null,
  );
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useWorkflowConfirm();

  const loadBinding = useCallback(async () => {
    try {
      setBinding(await api.workflows.getQueueWorkflow(queue));
      setLoaded(true);
      setLoadError("");
    } catch (err) {
      setLoaded(false);
      setLoadError(errorText(err));
    }
  }, [queue, api]);
  useEffect(() => {
    void Promise.resolve().then(loadBinding);
  }, [loadBinding]);
  useEffect(() => {
    void api.workflows.listWorkflowImages().then(setImages, (err) => setError(errorText(err)));
  }, [api]);
  const sorted = useMemo(() => sortImages(images), [images]);

  const refused = async (err: unknown) => {
    const refusal = classifyBindError(err);
    switch (refusal.kind) {
      case "conflict":
        await loadBinding();
        setNotice("The binding changed elsewhere. It was reloaded; review and retry.");
        return;
      case "pools":
        setEmptyPools({ names: refusal.names, message: refusal.message });
        onPoolsMissing(refusal.names);
        return;
      case "secrets":
        setMissingSecrets({ names: refusal.names, message: refusal.message });
        onSecretsMissing({ names: refusal.names, message: refusal.message });
        return;
      case "other":
        setError(refusal.message);
    }
  };

  const reset = () => {
    setError("");
    setNotice("");
    setEmptyPools(null);
    setMissingSecrets(null);
    onPoolsMissing(null);
    onSecretsMissing(null);
  };
  const stillEmpty = stillEmptyPools(emptyPools?.names ?? [], pools);

  // The chosen image previews its statuses; otherwise the bound one shows, as NAME:DIGEST.
  const shownKey = ref || (binding ? `${binding.name}:${binding.digest}` : "");
  const definition = useShownDefinition(shownKey);

  const bind = async () => {
    reset();
    setBusy(true);
    try {
      setBinding(await api.workflows.setQueueWorkflow(queue, ref, binding?.revision ?? 0));
      setRef("");
      onChanged();
    } catch (err) {
      await refused(err);
    } finally {
      setBusy(false);
    }
  };

  const clear = () => {
    if (!binding) return;
    confirm({
      title: `Clear the workflow of ${queue}?`,
      description:
        "New tasks in this queue become flexible tasks. Tasks already running the workflow keep it.",
      action: "Clear binding",
      run: () => {
        reset();
        setBusy(true);
        api.workflows
          .clearQueueWorkflow(queue, binding.revision)
          .then(() => {
            setBinding(null);
            onChanged();
          }, refused)
          .finally(() => setBusy(false));
      },
    });
  };

  return (
    <section className="flex min-w-0 flex-col gap-2.5" aria-label={`Workflow ${queue}`}>
      <span className={LABEL}>Workflow</span>
      {loaded && <BindingSummary binding={binding} images={images} />}
      {loadError && <LoadError error={loadError} onRetry={() => void loadBinding()} />}
      <BindControls
        images={sorted}
        value={ref}
        onChoose={setRef}
        canBind={!busy && !!ref && loaded}
        canClear={!busy && loaded && !!binding}
        onBind={() => void bind()}
        onClear={clear}
      />
      <BindFeedback
        notice={notice}
        error={error}
        emptyPools={emptyPools && { message: emptyPools.message, names: stillEmpty }}
        missingSecrets={missingSecrets}
        onOpenTab={onOpenTab}
      />
      {shownKey && (
        <StatusPreview
          title={ref ? `Statuses of ${ref}` : "Statuses"}
          definition={definition.key === shownKey ? definition : null}
          pools={pools}
          onPool={() => onOpenTab("pools")}
        />
      )}
      {dialog}
    </section>
  );
}

interface ShownDefinition {
  key: string;
  value: WorkflowImageDefinition | null;
  error: string;
}

/** The statuses of the image named NAME:REF, loaded as the key changes. */
function useShownDefinition(shownKey: string): ShownDefinition {
  const api = useApi();
  const [definition, setDefinition] = useState<ShownDefinition>({
    key: "",
    value: null,
    error: "",
  });
  useEffect(() => {
    if (!shownKey) return;
    let alive = true;
    const colon = shownKey.lastIndexOf(":");
    api.workflows.getWorkflowImage(shownKey.slice(0, colon), shownKey.slice(colon + 1)).then(
      (manifest) => {
        if (alive) setDefinition({ key: shownKey, value: manifest.definition, error: "" });
      },
      (err) => {
        if (alive) setDefinition({ key: shownKey, value: null, error: errorText(err) });
      },
    );
    return () => {
      alive = false;
    };
  }, [shownKey, api]);
  return definition;
}

/** The bound image with its version, digest, and followed tag. */
function BindingSummary({
  binding,
  images,
}: {
  binding: QueueWorkflow | null;
  images: WorkflowImageListing[];
}) {
  if (!binding) return <span className={EMPTY}>No workflow</span>;
  return (
    <p data-testid="binding" className="text-body-sm">
      {/* Tasks and the Library both open on the active host. */}
      <Link
        className="font-semibold text-primary hover:underline"
        to={`/library/workflows/${encodeURIComponent(binding.name)}/${encodeURIComponent(bindingRef(binding, images))}`}
      >
        {binding.name}
      </Link>{" "}
      {binding.version}{" "}
      <span className={MONO} title={binding.digest}>
        {binding.digest.slice(0, 12)}
      </span>
      {binding.tag && (
        <>
          {" "}
          follows <span className={MONO}>{binding.tag}</span>
        </>
      )}
    </p>
  );
}

function LoadError({ error, onRetry }: { error: string; onRetry: () => void }) {
  return (
    <div role="alert" className="flex items-center gap-2 text-ui text-destructive">
      <span>{error}</span>
      <Button type="button" size="sm" variant="secondary" className="h-6.5" onClick={onRetry}>
        Retry
      </Button>
    </div>
  );
}

/** The image select with Bind and Clear. */
function BindControls({
  images,
  value,
  onChoose,
  canBind,
  canClear,
  onBind,
  onClear,
}: {
  images: WorkflowImageListing[];
  value: string;
  onChoose: (ref: string) => void;
  canBind: boolean;
  canClear: boolean;
  onBind: () => void;
  onClear: () => void;
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      <SelectShell
        aria-label="Workflow image"
        value={value}
        className="h-6.5 w-auto text-ui md:text-ui"
        onChange={(event) => onChoose(event.target.value)}
      >
        <option value="">Choose an image</option>
        {images.map((image) => (
          <option key={`${image.name}:${image.tag}`} value={`${image.name}:${image.tag}`}>
            {image.name}:{image.tag}
          </option>
        ))}
      </SelectShell>
      <Button type="button" size="sm" className="h-6.5" disabled={!canBind} onClick={onBind}>
        Bind
      </Button>
      <Button
        type="button"
        size="sm"
        variant="secondary"
        className="h-6.5"
        disabled={!canClear}
        onClick={onClear}
      >
        Clear
      </Button>
    </div>
  );
}

type Missing = { names: string[]; message: string } | null;

/** What the last bind or clear said: a notice, an error, or what it lacks. */
function BindFeedback({
  notice,
  error,
  emptyPools,
  missingSecrets,
  onOpenTab,
}: {
  notice: string;
  error: string;
  /** The pools a refused bind named that still have no agents. */
  emptyPools: Missing;
  missingSecrets: Missing;
  onOpenTab: (tab: "pools" | "environment") => void;
}) {
  return (
    <>
      {notice && (
        <p role="status" className="text-ui text-muted-foreground">
          {notice}
        </p>
      )}
      {error && (
        <p role="alert" className="text-ui text-destructive">
          {error}
        </p>
      )}
      {emptyPools && emptyPools.names.length > 0 && (
        <MissingNotice
          label="Pools needed"
          message={emptyPools.message}
          names={emptyPools.names}
          action="Open Pools"
          onOpen={() => onOpenTab("pools")}
        />
      )}
      {missingSecrets && (
        <MissingNotice
          label="Secrets needed"
          message={missingSecrets.message}
          names={missingSecrets.names}
          action="Open Environment"
          onOpen={() => onOpenTab("environment")}
        />
      )}
    </>
  );
}

/** What a refused bind still lacks, with a way to the tab that fixes it. */
function MissingNotice({
  label,
  message,
  names,
  action,
  onOpen,
}: {
  label: string;
  message: string;
  names: string[];
  action: string;
  onOpen: () => void;
}) {
  return (
    <div role="alert" aria-label={label} className="flex flex-col gap-1 text-ui text-destructive">
      <p>{message}</p>
      <p className="font-mono">{names.join(", ")}</p>
      <Button
        type="button"
        size="sm"
        variant="secondary"
        className="h-6.5 self-start"
        onClick={onOpen}
      >
        {action}
      </Button>
    </div>
  );
}

/** The statuses of the shown image; `null` while they load. */
function StatusPreview({
  title,
  definition,
  pools,
  onPool,
}: {
  title: string;
  definition: ShownDefinition | null;
  pools: AgentPool[];
  onPool: () => void;
}) {
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <span className={LABEL}>{title}</span>
      {!definition ? (
        <span className={EMPTY}>Loading statuses…</span>
      ) : definition.value ? (
        <WorkflowStatusMap definition={definition.value} pools={pools} onPool={onPool} />
      ) : (
        <p role="alert" className="text-ui text-destructive">
          {definition.error}
        </p>
      )}
    </section>
  );
}
