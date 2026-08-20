import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import {
  ApiError,
  createAgent,
  imageManifestGet,
  listImages,
  startAgent,
  type CreateAgentSpec,
  type ImageManifest,
  type ImageRow,
} from "@/lib/api";
import { resolveDaemon, unresolvedDaemon, type Daemon } from "@/lib/daemons";
import { HARNESSES, ImageCombobox } from "@/components/AgentFormFields";
import { EditablePresetCombobox } from "@/components/EditablePresetCombobox";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  rememberRuntimePreset,
  runtimePresetOptions,
} from "@/lib/runtimePresets";

const LOCAL_HOST_VALUE = "__local__";

export interface CreateAgentDialogProps {
  open: boolean;
  hostId: string;
  imageRef?: string;
  hosts: { id: string; label: string; revision?: string }[];
  onOpenChange: (open: boolean) => void;
  onCreated: (hostId: string, name: string) => void;
}

function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : String(error);
}

export function CreateAgentDialog(props: CreateAgentDialogProps) {
  if (!props.open) return null;
  return (
    <CreateAgentDialogForm
      key={`${props.hostId}\u0000${props.imageRef ?? ""}`}
      {...props}
    />
  );
}

function CreateAgentDialogForm({
  open,
  hostId,
  imageRef = "",
  hosts,
  onOpenChange,
  onCreated,
}: CreateAgentDialogProps) {
  const [host, setHost] = useState(hostId);
  const selectedHostRevision =
    hosts.find((entry) => entry.id === host)?.revision ?? "missing";
  const [target, setTarget] = useState<Daemon | null>(null);
  const [resolvedRevision, setResolvedRevision] = useState("");
  const [images, setImages] = useState<ImageRow[]>([]);
  const [image, setImage] = useState(imageRef);
  const [manifest, setManifest] = useState<ImageManifest | null>(null);
  const [manifestKey, setManifestKey] = useState("");
  const runtimeInitializedForTarget = useRef("");
  const [name, setName] = useState("");
  const [cwd, setCwd] = useState("");
  const [startNow, setStartNow] = useState(true);
  const [interactive, setInteractive] = useState(false);
  const [autopilot, setAutopilot] = useState(true);
  const [advanced, setAdvanced] = useState(false);
  const [harness, setHarness] = useState("");
  const [model, setModel] = useState("");
  const [effort, setEffort] = useState("");
  const [env, setEnv] = useState("");
  const [busy, setBusy] = useState(false);
  const [loadingImages, setLoadingImages] = useState(true);
  const [loadingManifest, setLoadingManifest] = useState(imageRef !== "");
  const [formError, setFormError] = useState("");
  const [startError, setStartError] = useState("");
  const [created, setCreated] = useState<{ host: string; name: string } | null>(null);

  const targetIsCurrent = resolvedRevision === selectedHostRevision;
  const targetIsUsable = host === "" || target !== null;
  const expectedManifestKey = `${selectedHostRevision}\u0000${image}`;
  const runtimeTargetKey = `${host}\u0000${image}`;
  const manifestIsCurrent =
    image !== "" && manifestKey === expectedManifestKey && manifest !== null;
  const currentManifest = manifestIsCurrent ? manifest : null;
  const bare = currentManifest?.bare === true;
  const defaultHarness=currentManifest?.schema_version===1?currentManifest.harness?.type??"":"";
  const defaultModel=currentManifest?.schema_version===1?currentManifest.harness?.model??"":"";
  const defaultEffort=currentManifest?.schema_version===1?currentManifest.harness?.effort??"":"";
  const modelPresets = runtimePresetOptions(
    harness,
    "models",
    [defaultModel,model],
  );
  const effortPresets = runtimePresetOptions(
    harness,
    "efforts",
    [defaultEffort,effort],
  );

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const nextTarget = host ? await resolveDaemon(host) : null;
        if (host && !nextTarget) throw new Error(`host ${host} was not found`);
        const result = await listImages(nextTarget);
        if (!alive) return;
        setTarget(nextTarget);
        setResolvedRevision(selectedHostRevision);
        setImages(result.images ?? []);
        setFormError("");
      } catch (error) {
        if (!alive) return;
        setTarget(null);
        setResolvedRevision(selectedHostRevision);
        setImages([]);
        setFormError(`Could not load images: ${errorMessage(error)}`);
      } finally {
        if (alive) setLoadingImages(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [host, selectedHostRevision]);

  useEffect(() => {
    if (!image || !targetIsCurrent || !targetIsUsable) return;
    let alive = true;
    void imageManifestGet(image, target)
      .then((next) => {
        if (!alive) return;
        setManifest(next);
        setManifestKey(expectedManifestKey);
        if (runtimeInitializedForTarget.current !== runtimeTargetKey) {
          runtimeInitializedForTarget.current = runtimeTargetKey;
          if(next.schema_version===1&&next.harness){setHarness(next.harness.type??"");setModel(next.harness.model??"");setEffort(next.harness.effort??"");setInteractive(next.harness.interactive);}
          if (next.bare) {
            setAutopilot(false);
          } else {
            setAutopilot(true);
          }
        }
      })
      .catch((error) => {
        if (!alive) return;
        setManifest(null);
        setManifestKey(expectedManifestKey);
        setFormError(`Could not load ${image}: ${errorMessage(error)}`);
      })
      .finally(() => {
        if (alive) setLoadingManifest(false);
      });
    return () => {
      alive = false;
    };
  }, [expectedManifestKey, image, target, targetIsCurrent, targetIsUsable]);

  const changeHost = (value: string) => {
    const next = value === LOCAL_HOST_VALUE ? "" : value;
    if (next === host) return;
    setHost(next);
    setTarget(null);
    setResolvedRevision("");
    setImages([]);
    setLoadingImages(true);
    setImage("");
    setManifest(null);
    setManifestKey("");
    setLoadingManifest(false);
    setFormError("");
  };

  const changeImage = (next: string) => {
    setImage(next);
    setManifest(null);
    setManifestKey("");
    setLoadingManifest(next !== "");
    setFormError("");
  };

  const submit = async () => {
    if (!image || !targetIsCurrent || !targetIsUsable || !manifestIsCurrent) {
      setFormError("Wait for the selected host and image to finish loading");
      return;
    }
    const spec: CreateAgentSpec = { image, interactive:bare?true:interactive, loop:bare?false:autopilot };
    if (name.trim()) spec.name = name.trim();
    if (cwd.trim()) spec.cwd = cwd.trim();
    if (harness.trim()&&harness.trim()!==defaultHarness) spec.harness=harness.trim();
    if (model.trim()&&model.trim()!==defaultModel) spec.model=model.trim();
    if (effort.trim()&&effort.trim()!==defaultEffort) spec.effort=effort.trim();
    if (env.trim()) spec.env = env.trim();

    setBusy(true);
    setFormError("");
    setStartError("");
    try {
      const result = await createAgent(spec, target);
      if (model.trim()&&model.trim()!==defaultModel) {
        rememberRuntimePreset(harness, "models", model);
      }
      if (effort.trim()&&effort.trim()!==defaultEffort) {
        rememberRuntimePreset(harness, "efforts", effort);
      }
      const nextCreated = { host, name: result.name };
      setCreated(nextCreated);
      onCreated(host, result.name);
      if (!startNow) {
        onOpenChange(false);
        return;
      }
      try {
        await startAgent(result.name, target);
        onOpenChange(false);
      } catch (error) {
        const message = errorMessage(error);
        setStartError(
          `Agent created but could not be started: ${message}. Check the host and retry start.`,
        );
      }
    } catch (error) {
      const message = errorMessage(error);
      setFormError(message);
      toast.error(`create failed: ${message}`);
    } finally {
      setBusy(false);
    }
  };

  const retryStart = async () => {
    if (!created) return;
    setBusy(true);
    setStartError("");
    try {
      await startAgent(created.name, target);
      onOpenChange(false);
    } catch (error) {
      setStartError(
        `Agent is still stopped: ${errorMessage(error)}. Check the host and retry start.`,
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>New agent</DialogTitle>
          <DialogDescription>
            Choose an image first, then decide independently how the agent should run.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="create-agent-host">Host</Label>
            <Select
              value={host || LOCAL_HOST_VALUE}
              onValueChange={changeHost}
              disabled={created !== null}
            >
              <SelectTrigger id="create-agent-host" className="h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {hosts.map((entry) => (
                  <SelectItem
                    key={entry.id || LOCAL_HOST_VALUE}
                    value={entry.id || LOCAL_HOST_VALUE}
                  >
                    {entry.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <Label htmlFor="create-agent-image">Image</Label>
              {bare && <Badge variant="secondary">Terminal only</Badge>}
            </div>
            <ImageCombobox
              id="create-agent-image"
              images={images}
              value={image}
              onChange={changeImage}
              ariaLabel="image"
            />
            {loadingImages && (
              <p className="text-xs text-muted-foreground">Loading built images…</p>
            )}
          </div>

          <div className="space-y-3 rounded border p-3">
            <div>
              <Label>Runtime</Label>
              <p className="text-xs text-muted-foreground">
                {currentManifest?.schema_version === 1
                  ? "Legacy image defaults can be overridden for this agent."
                  : "Runtime settings are independent of the selected image."}
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="create-agent-harness">Harness</Label>
                <select
                  id="create-agent-harness"
                  aria-label="harness"
                  value={harness}
                  onChange={(event) => setHarness(event.target.value)}
                  disabled={loadingManifest || created !== null}
                  className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30"
                >
                  {HARNESSES.map((value) => (
                    <option key={value} value={value}>{value}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-1">
                <Label htmlFor="create-agent-effort">Effort</Label>
                <EditablePresetCombobox
                  id="create-agent-effort"
                  ariaLabel="effort"
                  value={effort}
                  options={effortPresets}
                  onChange={setEffort}
                  placeholder="image default"
                  disabled={loadingManifest || created !== null}
                />
              </div>
              <div className="space-y-1 sm:col-span-2">
                <Label htmlFor="create-agent-model">Model</Label>
                <EditablePresetCombobox
                  id="create-agent-model"
                  ariaLabel="model"
                  value={model}
                  options={modelPresets}
                  onChange={setModel}
                  placeholder="image default"
                  disabled={loadingManifest || created !== null}
                />
              </div>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="create-agent-cwd">cwd</Label>
              <PathAutocomplete
                id="create-agent-cwd"
                value={cwd}
                onChange={setCwd}
                daemon={
                  host === ""
                    ? null
                    : targetIsCurrent && target
                      ? target
                      : unresolvedDaemon(
                          host,
                          hosts.find((entry) => entry.id === host)?.label,
                        )
                }
                placeholder="empty = managed workdir"
                aria-label="cwd"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="create-agent-name">name</Label>
              <Input
                id="create-agent-name"
                aria-label="name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="empty = generated"
                className="h-8"
              />
            </div>
          </div>

          <div className="space-y-3 rounded border p-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <Label htmlFor="create-agent-start">Start now</Label>
                <p className="text-xs text-muted-foreground">
                  Off creates the agent in stopped state.
                </p>
              </div>
              <Switch
                id="create-agent-start"
                checked={startNow}
                onCheckedChange={setStartNow}
                disabled={created !== null}
              />
            </div>
            <div className="flex items-center justify-between gap-3">
              <div>
                <Label htmlFor="create-agent-interactive">Interactive</Label>
                <p className="text-xs text-muted-foreground">Attach a terminal console.</p>
              </div>
              <Switch
                id="create-agent-interactive"
                checked={bare ? true : interactive}
                onCheckedChange={setInteractive}
                disabled={loadingManifest || bare || created !== null}
              />
            </div>
            <div className="flex items-center justify-between gap-3">
              <div>
                <Label htmlFor="create-agent-autopilot">Autopilot</Label>
                <p className="text-xs text-muted-foreground">
                  {bare
                    ? "A terminal-only image does not contain agent instructions or tools."
                    : "Enable loop and event-driven work."}
                </p>
              </div>
              <Switch
                id="create-agent-autopilot"
                checked={bare ? false : autopilot}
                onCheckedChange={setAutopilot}
                disabled={loadingManifest || bare || created !== null}
              />
            </div>
          </div>

          <Button
            variant="ghost"
            size="sm"
            type="button"
            onClick={() => setAdvanced((value) => !value)}
          >
            Advanced overrides
          </Button>
          {advanced && (
            <div className="rounded border p-3">
              <div className="space-y-1">
                <Label htmlFor="create-agent-env">env (K=V,K=V)</Label>
                <Input
                  id="create-agent-env"
                  aria-label="env (K=V,K=V)"
                  value={env}
                  onChange={(event) => setEnv(event.target.value)}
                  placeholder="empty = none"
                  className="h-8"
                />
              </div>
            </div>
          )}

          {formError && (
            <p role="alert" className="text-sm text-destructive">{formError}</p>
          )}
          {startError && (
            <div role="alert" className="space-y-2 rounded border border-destructive p-3">
              <p className="text-sm text-destructive">{startError}</p>
              <Button
                disabled={busy || !targetIsCurrent || !targetIsUsable}
                size="sm"
                onClick={() => void retryStart()}
              >
                Retry start
              </Button>
            </div>
          )}
        </div>

        <DialogFooter>
          {!created && (
            <Button
              disabled={
                busy
                || loadingImages
                || loadingManifest
                || !targetIsCurrent
                || !targetIsUsable
                || !image
                || !manifestIsCurrent
              }
              onClick={() => void submit()}
            >
              {busy ? "Creating…" : "Create agent"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
