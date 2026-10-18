import { useCallback, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { RunStateActions } from "@/components/AgentControls";
import { GoalHelp } from "@/components/GoalHelp";
import { useHarnessCatalog } from "@/hooks/useHarnessCatalog";
import { agentImageCancelOn, agentImageSetOn, agentPostOn, ApiError } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { useAgentName, useAgentStatus } from "@/lib/agent";
import type { Daemon } from "@/lib/daemons";
import type { AgentStatus } from "@/lib/types";
import { runtimePresetGroups } from "@/lib/runtimePresets";
import { serverPath } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";
import { BLOCKS, blockOfKey, fieldOfKey, filterBlocks, type ConfigBlock, type ConfigField } from "./configuration/registry";
import { useConfigDraft, type ConfigDraft } from "./configuration/useConfigDraft";
import { useConfigSources, type ConfigSources } from "./configuration/useConfigSources";
import { buildUnits, readBaseline, removedSecrets } from "./configuration/commits";
import { ConfigBlockCard, ConfigSaveBar, ConfigSearch, DraftField, FieldFrame } from "./configuration/ConfigControls";

// Operator-selectable harnesses. This is intentionally a SUBSET of the backend
// allowlist (internal/harness.Get() = claude | codex | opencode | cursor | stub): 'stub'
// is a test-only harness that the compose validator rejects, so it is omitted
// here to keep operators from picking a value that can't be applied. An agent
// already on 'stub' still shows it as its current value.
const HARNESS_OPTIONS = ["claude", "codex", "opencode", "cursor"] as const;

// A digest is a 64-hex-character hash: shown whole it dominates the identity
// row and wraps across lines. Keep enough of both ends to compare against a
// registry listing by eye; the untruncated value stays in the a11y tree.
function shortDigest(digest: string): string {
  return digest.length > 24 ? `${digest.slice(0, 14)}…${digest.slice(-8)}` : digest;
}

const field = (key: string) => fieldOfKey(key) as ConfigField;

export default function AgentConfigurationTab({
  target,
  refresh,
}: {
  target: Daemon | null;
  refresh: () => void;
}) {
  const name = useAgentName();
  const { status } = useAgentStatus();
  const targetId = target?.id;
  const targetLabel = target?.label;
  const targetBaseURL = target?.baseURL;
  const targetToken = target?.token;
  // Host registry refreshes replace the descriptor object even when its API
  // endpoint is unchanged. Keep a stable request target so those parent
  // renders cannot reload the sources over an in-progress draft.
  const requestTarget = useMemo<Daemon | null>(
    () => targetId === undefined ? null : {
      id: targetId,
      label: targetLabel ?? targetId,
      baseURL: targetBaseURL ?? "",
      token: targetToken ?? "",
    },
    [targetBaseURL, targetId, targetLabel, targetToken],
  );
  // One draft per scope (one agent on one host): a reconnect keeps the scope
  // and the draft; another agent or host starts clean.
  return (
    <ConfigurationPage
      key={`${targetId ?? ""}\u0000${name}`}
      target={requestTarget}
      name={name}
      status={status}
      refresh={refresh}
    />
  );
}

function ConfigurationPage({
  target,
  name,
  status,
  refresh,
}: {
  target: Daemon | null;
  name: string;
  status: AgentStatus | null;
  refresh: () => void;
}) {
  const sources = useConfigSources(target, name, status);
  const { view, imageStatus } = sources;
  const baseline = useMemo(() => readBaseline({
    view,
    imageStatus,
    script: sources.script.value,
    retention: sources.retention.value,
    secrets: sources.secrets.value,
  }), [view, imageStatus, sources.script.value, sources.retention.value, sources.secrets.value]);
  const units = useMemo(() => buildUnits(target, name), [target, name]);
  const [query, setQuery] = useState("");
  const [changedOnly, setChangedOnly] = useState(false);

  // Older daemons omit the budget projection; then the block does not exist.
  const available = useMemo(
    () => BLOCKS.filter((block) => block.id !== "budgets" || !view || view.budget),
    [view],
  );
  // A save that stops on a field shows that field: search is cleared when it
  // hides the field's block, and focus moves once the block has rendered.
  const onFocus = useCallback((key: string) => {
    if (!document.getElementById(key)) {
      setQuery("");
      setChangedOnly(false);
    }
    window.setTimeout(() => {
      const element = document.getElementById(key);
      element?.scrollIntoView?.({ block: "center" });
      element?.focus();
    }, 0);
  }, []);
  const reload = useCallback(async () => {
    await sources.reloadAll();
    refresh();
  }, [sources, refresh]);
  const draft = useConfigDraft({ baseline, units, reload, onFocus });

  const dirtyByBlock = new Map<string, number>();
  for (const key of draft.dirtyKeys) {
    const id = blockOfKey(key);
    dirtyByBlock.set(id, (dirtyByBlock.get(id) ?? 0) + 1);
  }
  const filtered = filterBlocks(query, available);
  const visible = filtered.blocks.filter((block) => !changedOnly || dirtyByBlock.has(block.id));
  const filtering = query.trim() !== "" || changedOnly;

  const stopped = status
    ? status.state === "stopped"
    : view?.enabled === false || (view?.enabled === undefined && view?.state === "stopped");

  return (
    <div className="mx-auto max-w-5xl space-y-4 px-4 pt-4 pb-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h2 className="font-semibold">Configuration</h2>
          {view && (
            <p className="text-sm">
              <span className="text-muted-foreground">Master switch</span>{" "}
              <span data-testid="master-switch-state" className="font-medium">
                {view.enabled === false ? "Disabled" : "Enabled"}
              </span>
            </p>
          )}
        </div>
        <div className="space-y-1">
          <RunStateActions name={name} refresh={() => void reload()} />
          <p className="text-xs text-muted-foreground">Run actions apply immediately.</p>
        </div>
      </header>
      <ConfigSearch
        query={query}
        onQuery={setQuery}
        changedOnly={changedOnly}
        onChangedOnly={setChangedOnly}
        dirtyCount={draft.dirtyKeys.length}
        chips={available.map((block) => ({ id: block.id, title: block.title, dirty: dirtyByBlock.get(block.id) ?? 0 }))}
        resultText={filtering ? `${visible.length} of ${available.length} blocks match` : ""}
      />
      {!view ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : visible.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center">
          <p className="text-sm font-medium">{query.trim() ? `No settings match “${query.trim()}”` : "No blocks have unsaved changes"}</p>
          <p className="mt-1 text-xs text-muted-foreground">Unsaved changes in hidden blocks are kept.</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => { setQuery(""); setChangedOnly(false); }}>
            Clear search
          </Button>
        </div>
      ) : (
        visible.map((block) => (
          <BlockView
            key={block.id}
            block={block}
            draft={draft}
            matched={filtered.matched}
            dirty={dirtyByBlock.get(block.id) ?? 0}
            sources={sources}
            target={target}
            name={name}
            stopped={stopped}
            refresh={refresh}
          />
        ))
      )}
      <ConfigSaveBar
        draft={draft}
        blockTitles={available.filter((block) => dirtyByBlock.has(block.id)).map((block) => block.title)}
      />
    </div>
  );
}

interface BlockProps {
  block: ConfigBlock;
  draft: ConfigDraft;
  matched: Set<string>;
  dirty: number;
  sources: ConfigSources;
  target: Daemon | null;
  name: string;
  stopped: boolean;
  refresh: () => void;
}

function BlockView(props: BlockProps) {
  const { block, draft, matched, dirty } = props;
  const plain = (key: string) => <DraftField field={field(key)} draft={draft} matched={matched.has(key)} />;
  let body;
  switch (block.id) {
    case "general": body = <GeneralBlock {...props} />; break;
    case "runtime": body = <RuntimeBlock {...props} />; break;
    case "loop":
      body = <>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">{plain("interval")}{plain("timeout")}{plain("hard-timeout")}</div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{plain("on-timeout")}{plain("on-error")}</div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{plain("max-idle")}{plain("ai-stall-timeout")}</div>
      </>;
      break;
    case "goal":
      body = <>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {plain("goal-enabled")}{plain("goal-wait-customer-timeout")}{plain("goal-delivery-cooldown")}
        </div>
        <ReadOnly draft={draft} matched={matched} fieldKey="current-goal-task">
          <Input id="current-goal-task" value={props.sources.view?.current_goal_task_key || "No current goal"} disabled aria-describedby="current-goal-task-help" />
        </ReadOnly>
      </>;
      break;
    case "messages":
      body = <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{plain("messages-batch")}{plain("messages-max-queue")}</div>;
      break;
    case "budgets": body = <BudgetsBlock {...props} />; break;
    case "environment": body = <EnvironmentBlock {...props} />; break;
    case "retention": body = <RetentionBlock {...props} />; break;
  }
  return (
    <ConfigBlockCard block={block} dirtyCount={dirty} action={block.id === "goal" ? <GoalHelp /> : undefined}>
      {body}
    </ConfigBlockCard>
  );
}

function ReadOnly({ draft, matched, fieldKey, children }: { draft: ConfigDraft; matched: Set<string>; fieldKey: string; children: React.ReactNode }) {
  return <FieldFrame field={field(fieldKey)} draft={draft} matched={matched.has(fieldKey)}>{children}</FieldFrame>;
}

// Actions run at once and are never part of Save; the row says so.
function ActionRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2 border-t pt-4">
      <span className="text-xs text-muted-foreground">Actions apply immediately, not part of Save:</span>
      {children}
    </div>
  );
}

function GeneralBlock({ draft, matched, sources, target, name, stopped, refresh }: BlockProps) {
  const { view, imageStatus, imageError, images, inventoryLoading, inventoryError } = sources;
  const [copied, setCopied] = useState(false);
  const [imageBusy, setImageBusy] = useState(false);
  const imageHref = (ref: string) => {
    const split = ref.lastIndexOf(":");
    const base = serverPath(target?.id ?? "", "images");
    return split < 0 ? base : `${base}/${encodeURIComponent(ref.slice(0, split))}/${encodeURIComponent(ref.slice(split + 1))}`;
  };
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
  const imageField = field("image");
  const cwdField = field("cwd");
  const cwdError = draft.fieldErrors.cwd ?? "";
  return (
    <>
      {imageError && <p role="alert" className="text-sm text-destructive">Image status unavailable: {imageError}</p>}
      {inventoryError && <p role="alert" className="text-sm text-destructive">Image inventory unavailable: {inventoryError}</p>}
      {imageStatus && (
        <div className="grid gap-4 text-sm sm:grid-cols-2">
          {([{ label: stopped ? "Activated version" : "Current", value: imageStatus.current }, { label: "Next iteration", value: imageStatus.next }] as const).map(({ label, value }) => value && (
            <div key={label} role="group" aria-label={label} className="space-y-0.5">
              <div className="font-medium">{label}</div>
              <Link className="font-mono text-primary hover:underline" to={imageHref(value.ref)}>{value.ref}</Link>
              <div>{value.error ? "Version unavailable" : value.image_version || "Version not specified"}</div>
              <div className="font-mono text-xs break-all">{value.digest || "Digest unavailable"}</div>
              {value.error && <p role="alert" className="mt-1 text-destructive">{value.error}</p>}
            </div>
          ))}
          {imageStatus.next && (
            <div className="space-y-1 sm:col-span-2">
              <p className="text-xs text-muted-foreground">{imageStatus.next.reason === "pending" ? "Explicit pending assignment" : imageStatus.next.reason === "ref_moved" ? "Content the active ref points at now" : "Current pinned image"}</p>
              {!imageStatus.next.error && imageStatus.next.digest && imageStatus.next.digest !== imageStatus.current.digest && <p role="status">Update on next iteration</p>}
              <p className="text-xs text-muted-foreground">This preview is not a reservation; the image may change before launch.</p>
            </div>
          )}
          {imageStatus.pending.ref && <div className="sm:col-span-2">Pending: <Link className="font-mono text-primary hover:underline" to={imageHref(imageStatus.pending.ref)}>{imageStatus.pending.ref}</Link></div>}
          {imageStatus.pending.error && <p role="alert" className="text-destructive sm:col-span-2">{imageStatus.pending.error}</p>}
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <FieldFrame field={imageField} draft={draft} matched={matched.has("image")}>
          <Select
            disabled={!imageStatus || inventoryLoading || !!inventoryError}
            value={draft.values.image ?? ""}
            onValueChange={(value) => draft.setField("image", String(value ?? ""))}
          >
            <SelectTrigger id="image" aria-label="Agent image" aria-describedby={draft.fieldErrors.image ? "image-help image-error" : "image-help"} className="w-full">
              <SelectValue placeholder="Select image" />
            </SelectTrigger>
            <SelectContent>
              {images.map((item) => {
                const ref = `${item.name}:${item.tag}`;
                return <SelectItem key={ref} value={ref}>{ref}</SelectItem>;
              })}
            </SelectContent>
          </Select>
        </FieldFrame>
        <ReadOnly draft={draft} matched={matched} fieldKey="image-digest">
          <div className="flex min-h-9 flex-wrap items-center gap-2 text-sm">
            {view?.digest ? (
              <>
                <span data-testid="image-digest" aria-hidden="true" title={view.digest} className="font-mono text-xs break-all">{shortDigest(view.digest)}</span>
                <span className="sr-only">{view.digest}</span>
              </>
            ) : <span>—</span>}
          </div>
        </ReadOnly>
      </div>
      <div className="text-sm">
        <span className="text-muted-foreground">Image</span> <span className="break-all">{view?.image}</span>
      </div>
      {stopped ? (
        <FieldFrame field={cwdField} draft={draft} matched={matched.has("cwd")}>
          <PathAutocomplete
            id="cwd"
            value={draft.values.cwd ?? ""}
            onChange={(value) => draft.setField("cwd", value)}
            daemon={target}
            aria-label="Working directory"
          />
        </FieldFrame>
      ) : (
        <div data-field="cwd" className="space-y-1 text-sm">
          <div className="font-medium">Working directory</div>
          <p className="break-all">{view?.cwd || "—"}</p>
          {cwdError && <p role="alert" className="text-xs text-destructive">{cwdError}</p>}
          <p className="text-xs text-muted-foreground">Stop the agent before changing its working directory.</p>
        </div>
      )}
      {(view?.digest || imageStatus?.pending.ref) && (
        <ActionRow>
          {view?.digest && <Button size="sm" variant="outline" onClick={() => void copyDigest(view.digest)}>Copy digest</Button>}
          {copied && <span role="status" className="text-xs text-muted-foreground">Digest copied</span>}
          {imageStatus?.pending.error && (
            <Button size="sm" variant="outline" disabled={imageBusy} onClick={() => void imageAction("image assignment", () => agentImageSetOn(target, name, imageStatus.pending.ref))}>
              Retry
            </Button>
          )}
          {imageStatus?.pending.ref && (
            <Button size="sm" variant="outline" disabled={imageBusy} onClick={() => void imageAction("pending image cancellation", () => agentImageCancelOn(target, name))}>
              Cancel pending
            </Button>
          )}
        </ActionRow>
      )}
    </>
  );
}

function RuntimeBlock({ draft, matched, sources, target }: BlockProps) {
  const view = sources.view!;
  const harness = draft.values.harness ?? view.harness;
  const catalog = useHarnessCatalog(target, harness);
  const modelPresets = runtimePresetGroups({ harness, field: "models", catalog, extras: [view.model] });
  const effortPresets = runtimePresetGroups({ harness, field: "efforts", catalog, model: draft.values.model, extras: [view.effort] });
  // The operator subset, plus the agent's current harness if it falls outside
  // that subset, so the select never shows blank for an out-of-list value.
  const harnessOptions = (HARNESS_OPTIONS as readonly string[]).includes(view.harness)
    ? HARNESS_OPTIONS
    : [...HARNESS_OPTIONS, view.harness];
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <DraftField field={field("harness")} draft={draft} matched={matched.has("harness")} options={harnessOptions} />
      <DraftField field={field("interactive")} draft={draft} matched={matched.has("interactive")} />
      <DraftField field={field("model")} draft={draft} matched={matched.has("model")} presets={modelPresets} />
      <DraftField field={field("effort")} draft={draft} matched={matched.has("effort")} presets={effortPresets} />
    </div>
  );
}

const PERIODS = [["hour", "Hour"], ["day", "Day"], ["week", "Week"], ["month", "Month"]] as const;

function BudgetsBlock({ draft, matched, sources }: BlockProps) {
  const budget = sources.view?.budget;
  if (!budget) return null;
  const exhausted = budget.exhausted ?? [];
  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {PERIODS.map(([period]) => {
          const key = `budget-${period}`;
          const spent = budget[`${period}_spent_usd`];
          const limit = Number(draft.values[key] ?? 0);
          const helper = limit === 0 ? `Spent ${spent.toFixed(2)} · Unlimited` : `Spent ${spent.toFixed(2)} of ${limit.toFixed(2)}`;
          return <DraftField key={key} field={{ ...field(key), helper }} draft={draft} matched={matched.has(key)} />;
        })}
      </div>
      {exhausted.length > 0 && <p role="status" className="text-sm text-destructive">Out of budget: {exhausted.join(", ")}</p>}
    </>
  );
}

function EnvironmentBlock({ draft, matched, sources }: BlockProps) {
  const { script, secrets } = sources;
  const removed = removedSecrets(draft.values["secrets-remove"] ?? "");
  const toggle = (key: string) => {
    const next = removed.includes(key) ? removed.filter((k) => k !== key) : [...removed, key];
    draft.setField("secrets-remove", next.join("\n"));
  };
  const listField = field("secrets-remove");
  return (
    <>
      {script.error && <p role="alert" className="text-sm text-destructive">Shell script unavailable: {script.error}</p>}
      <DraftField field={field("shell-script")} draft={draft} matched={matched.has("shell-script")} />
      {/* SECURITY: secrets are write-only. The list holds key names only
          (the daemon never returns values); a new value lives only in the
          page draft while typing, is sent in a POST body, never in a URL, and
          is dropped once the daemon acknowledges it. */}
      {secrets.error && <p role="alert" className="text-sm text-destructive">Secrets unavailable: {secrets.error}</p>}
      <FieldFrame field={listField} draft={draft} matched={matched.has("secrets-remove")}>
        <div id="secrets-remove" tabIndex={-1} className="flex flex-wrap items-center gap-1.5">
          {(secrets.value ?? []).map((key) => {
            const pending = removed.includes(key);
            return (
              <Badge key={key} variant={pending ? "destructive" : "secondary"} className="gap-1">
                <span className={cn(pending && "line-through")}>{key}</span>
                {pending && <span>Will be removed</span>}
                <button
                  type="button"
                  className="ml-1 text-xs hover:text-destructive"
                  aria-label={pending ? `Keep ${key}` : `Remove ${key}`}
                  onClick={() => toggle(key)}
                >
                  {pending ? "Undo" : "×"}
                </button>
              </Badge>
            );
          })}
          {secrets.value?.length === 0 && (
            <span className="text-sm text-muted-foreground">Add a secret key and value to make it available to the agent.</span>
          )}
        </div>
      </FieldFrame>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField field={field("secret-key")} draft={draft} matched={matched.has("secret-key")} />
        <DraftField field={field("secret-value")} draft={draft} matched={matched.has("secret-value")} />
      </div>
    </>
  );
}

const reason = (e: unknown) => (e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e));

function RetentionBlock({ draft, matched, sources, target, name }: BlockProps) {
  const policy = sources.retention.value;
  // Prune is the one destructive action on this page: it is confirmed first,
  // it cannot be submitted twice while in flight, and a failure is reported
  // in place so the operator can read the reason and retry.
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pruning, setPruning] = useState(false);
  const [pruneError, setPruneError] = useState("");
  const prune = async () => {
    setPruning(true);
    setPruneError("");
    try {
      await agentPostOn(target, name, "prune", { "dry-run": false });
      await sources.reloadAll();
    } catch (e) {
      setPruneError(reason(e));
    }
    setPruning(false);
  };
  const inherits = (def: number | undefined) => (def === undefined ? "" : `0 = server default (${def}).`);
  return (
    <>
      {sources.retention.error && <p role="alert" className="text-sm text-destructive">Retention policy unavailable: {sources.retention.error}</p>}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField field={{ ...field("keep-iterations"), helper: inherits(policy?.default?.keep_iterations) }} draft={draft} matched={matched.has("keep-iterations")} />
        <DraftField field={{ ...field("keep-days"), helper: inherits(policy?.default?.keep_days) }} draft={draft} matched={matched.has("keep-days")} />
      </div>
      {policy && (
        <p className="text-xs text-muted-foreground">
          Effective: {policy.keep_iterations} iterations · {policy.keep_days} days · max_bytes {policy.max_bytes} (0 = unlimited).
          Pruned iterations are archived with their database rows first.
        </p>
      )}
      <ActionRow>
        <Button size="sm" variant="outline" onClick={() => void guard("prune (dry-run)", () => agentPostOn(target, name, "prune", { "dry-run": true }), { verb: "ok" })}>
          Preview cleanup
        </Button>
        <Button size="sm" variant="destructive" disabled={pruning} onClick={() => setConfirmOpen(true)}>Prune now</Button>
      </ActionRow>
      {pruneError && <p role="alert" className="text-xs text-destructive">{pruneError}</p>}
      {/* Closed means unmounted, so no stray Cancel/confirm button sits in
          the page waiting to be matched by a broader query. */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Prune retained data?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes data selected by the current retention policy. Review the preview first if you need to check what will be removed.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => { setConfirmOpen(false); void prune(); }}>
              Prune retained data
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
