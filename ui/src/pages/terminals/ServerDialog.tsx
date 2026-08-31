import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  addDaemon,
  clearDaemonToken,
  hasDaemonToken,
  setDaemonToken,
  updateDaemon,
  type DaemonMeta,
} from "@/lib/daemons";
import {
  hostPromptReply,
  hostProvision,
  hostSaveSsh,
  hostUpdate,
  isDesktop,
  onHostProvisionOutput,
  onHostState,
  type DesktopHostView,
  type HostOutputEvent,
} from "@/lib/desktop";
import {
  HOST_PROGRESS_STEPS,
  formatHostOperationError,
  hostStepForPhase,
  hostStepStates,
  isTerminalHostOutput,
  type HostOperationKind,
  type HostOperationStatus,
  type HostProgressStep,
} from "@/lib/hostProgress";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export interface ServerDialogProps {
  open: boolean;
  server?: DaemonMeta;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}

interface HostOperation {
  kind: HostOperationKind;
  status: HostOperationStatus;
  currentStep: HostProgressStep | null;
  error: string | null;
}

function idleOperation(kind: HostOperationKind): HostOperation {
  return {
    kind,
    status: "idle",
    currentStep: null,
    error: null,
  };
}

const INSTALL_REQUIREMENTS = new Set([
  "Linux",
  "x86_64",
  "writable ~/.local",
  "flock",
  "python3",
]);

function hostMeta(view: DesktopHostView): DaemonMeta {
  return {
    id: view.id,
    label: view.label,
    baseURL: view.base_url,
    kind: view.kind,
    state: view.state,
    sshAlias: view.ssh_alias,
    phase: view.phase,
    platform: view.platform,
    arch: view.arch,
    prerequisites: view.prerequisites,
    message: view.message,
    lastDaemonVersion: view.last_daemon_version,
  };
}

function preflightView(event: HostOutputEvent): Partial<DesktopHostView> | null {
  if (event.stream !== "stdout" || !event.text.trim().startsWith("{")) return null;
  try {
    const value = JSON.parse(event.text) as {
      platform?: string;
      arch?: string;
      prerequisites?: string[];
    };
    if (!value.platform && !value.arch) return null;
    return {
      platform: value.platform ?? "",
      arch: value.arch ?? "",
      prerequisites: value.prerequisites ?? [],
    };
  } catch {
    return null;
  }
}

export function ServerDialog({
  open,
  server,
  onOpenChange,
  onSaved,
}: ServerDialogProps) {
  const editing = server !== undefined;
  const [transport, setTransport] = useState<"ssh" | "https">(
    server?.kind ?? (isDesktop() ? "ssh" : "https"),
  );
  const [label, setLabel] = useState(server?.label ?? "");
  const [sshAlias, setSshAlias] = useState(server?.sshAlias ?? "");
  const [baseURL, setBaseURL] = useState(server?.baseURL ?? "");
  const [token, setToken] = useState("");
  const [tokenStored, setTokenStored] = useState(false);
  const hostIdRef = useRef(server?.id ?? "");
  const [host, setHost] = useState<DesktopHostView | null>(null);
  const [operationId, setOperationId] = useState("");
  const operationIdRef = useRef("");
  const initialOperation = idleOperation(
    server?.kind === "ssh" && server.state === "ready" ? "update" : "provision",
  );
  const [operation, setOperation] = useState<HostOperation>(initialOperation);
  const operationRef = useRef<HostOperation>(initialOperation);
  const [output, setOutput] = useState<HostOutputEvent[]>([]);
  const [prompt, setPrompt] = useState<HostOutputEvent | null>(null);
  const [reply, setReply] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const onSavedRef = useRef(onSaved);

  const updateOperation = useCallback((
    update: HostOperation | ((current: HostOperation) => HostOperation),
  ) => {
    const current = operationRef.current;
    const next = typeof update === "function" ? update(current) : update;
    operationRef.current = next;
    setOperation(next);
  }, []);

  useEffect(() => {
    onSavedRef.current = onSaved;
  }, [onSaved]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    void (async () => {
      const nextTransport = server?.kind ?? (isDesktop() ? "ssh" : "https");
      try {
        const stored =
          server && nextTransport === "https" ? await hasDaemonToken(server.id) : false;
        if (cancelled) return;
        setTransport(nextTransport);
        setLabel(server?.label ?? "");
        setSshAlias(server?.sshAlias ?? "");
        setBaseURL(server?.baseURL ?? "");
        setToken("");
        setTokenStored(stored);
        hostIdRef.current = server?.id ?? "";
        setHost(null);
        setOperationId("");
        operationIdRef.current = "";
        const nextOperation = idleOperation(
          server?.kind === "ssh" && server.state === "ready"
            ? "update"
            : "provision",
        );
        operationRef.current = nextOperation;
        setOperation(nextOperation);
        setOutput([]);
        setPrompt(null);
        setReply("");
        setFormError(null);
      } catch (cause) {
        if (!cancelled) setFormError(String(cause));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, server]);

  useEffect(() => {
    if (!open || transport !== "ssh") return;
    const offState = onHostState((next) => {
      if (next.id !== hostIdRef.current) return;
      setHost((current) => ({
        ...next,
        platform: next.platform || current?.platform || "",
        arch: next.arch || current?.arch || "",
        prerequisites:
          next.prerequisites.length > 0
            ? next.prerequisites
            : current?.prerequisites ?? [],
      }));
      const phaseStep = hostStepForPhase(next.phase);
      if (next.state === "failed" && operationRef.current.status !== "idle") {
        updateOperation((current) => ({
          ...current,
          status: "failed",
          currentStep: phaseStep ?? current.currentStep ?? "connect",
          error: formatHostOperationError(next.message),
        }));
      } else if (
        next.state === "ready"
        && operationRef.current.status === "running"
      ) {
        updateOperation((current) => ({
          ...current,
          status: "succeeded",
          currentStep: "reconnect",
          error: null,
        }));
      } else if (phaseStep && operationRef.current.status === "running") {
        updateOperation((current) => ({
          ...current,
          currentStep: phaseStep,
        }));
      }
      onSavedRef.current();
    });
    const offOutput = onHostProvisionOutput((event) => {
      if (event.host_id !== hostIdRef.current) return;
      if (operationRef.current.status === "idle") return;
      if (
        operationIdRef.current
        && operationIdRef.current !== event.operation_id
      ) {
        return;
      }
      if (!operationIdRef.current) {
        operationIdRef.current = event.operation_id;
        setOperationId(event.operation_id);
      }
      if (event.stream === "phase") {
        const step = hostStepForPhase(event.text);
        if (step) {
          updateOperation((current) => ({
            ...current,
            currentStep: step,
          }));
        }
      } else {
        setOutput((current) => [...current, event].slice(-200));
      }
      if (event.prompt) setPrompt(event);
      const preflight = preflightView(event);
      if (preflight) {
        setHost((current) =>
          current ? { ...current, ...preflight } : current,
        );
      }
      if (isTerminalHostOutput(event.stream)) {
        updateOperation((current) => ({
          ...current,
          status: "failed",
          currentStep: current.currentStep ?? "connect",
          error: formatHostOperationError(event.text),
        }));
      }
    });
    return () => {
      offState();
      offOutput();
    };
  }, [open, transport, updateOperation]);

  const displayHost = useMemo<DaemonMeta | null>(() => {
    if (host) return hostMeta(host);
    return server?.kind === "ssh" ? server : null;
  }, [host, server]);

  const dirtySsh = editing && (
    label.trim() !== (server?.label ?? "")
    || sshAlias.trim() !== (server?.sshAlias ?? "")
  );
  const stepStates = hostStepStates(operation.currentStep, operation.status);
  const prerequisites = displayHost?.prerequisites ?? [];
  const installRequirements = prerequisites.filter((item) =>
    INSTALL_REQUIREMENTS.has(item)
  );
  const optionalTools = prerequisites.filter((item) =>
    !INSTALL_REQUIREMENTS.has(item)
  );
  const unsupported = (
    (!!displayHost?.platform && displayHost.platform !== "Linux")
    || (!!displayHost?.arch && displayHost.arch !== "x86_64")
    || installRequirements.length > 0
  );

  const beginOperation = (kind: HostOperationKind) => {
    operationIdRef.current = "";
    setOperationId("");
    setOutput([]);
    setPrompt(null);
    setReply("");
    setFormError(null);
    updateOperation({
      kind,
      status: "running",
      currentStep: "connect",
      error: null,
    });
  };

  const rememberOperationId = (id: string) => {
    if (operationIdRef.current) return;
    operationIdRef.current = id;
    setOperationId(id);
  };

  const failOperation = (cause: unknown) => {
    updateOperation((current) => ({
      ...current,
      status: "failed",
      currentStep: current.currentStep ?? "connect",
      error: formatHostOperationError(String(cause)),
    }));
  };

  const submitHttps = async () => {
    const cleanLabel = label.trim();
    const cleanURL = baseURL.trim();
    const cleanToken = token.trim();
    if (!cleanLabel) return setFormError("label is required");
    if (!cleanURL || !cleanURL.startsWith("http")) {
      return setFormError("base URL must start with http");
    }
    try {
      if (server) {
        await updateDaemon(server.id, { label: cleanLabel, baseURL: cleanURL });
        if (cleanToken) await setDaemonToken(server.id, cleanToken);
      } else {
        await addDaemon({ label: cleanLabel, baseURL: cleanURL, token: cleanToken });
      }
      onSaved();
      onOpenChange(false);
    } catch (cause) {
      setFormError(String(cause));
    }
  };

  const submitSsh = async () => {
    const cleanLabel = label.trim();
    const cleanAlias = sshAlias.trim();
    if (!cleanLabel) return setFormError("label is required");
    if (!cleanAlias) return setFormError("SSH alias is required");
    beginOperation("provision");
    try {
      const saved = await hostSaveSsh({
        id: server?.id,
        label: cleanLabel,
        ssh_alias: cleanAlias,
      });
      if (!saved) throw new Error("host_save_ssh is unavailable outside the desktop shell");
      hostIdRef.current = saved.id;
      setHost(saved);
      onSaved();
      const operation = await hostProvision(saved.id);
      if (!operation) throw new Error("host_provision is unavailable outside the desktop shell");
      rememberOperationId(operation.operation_id);
    } catch (cause) {
      failOperation(cause);
    }
  };

  const updateHost = async () => {
    if (!server) return;
    beginOperation("update");
    try {
      const operation = await hostUpdate(server.id);
      if (!operation) throw new Error("host_update is unavailable outside the desktop shell");
      rememberOperationId(operation.operation_id);
    } catch (cause) {
      failOperation(cause);
    }
  };

  const sendReply = async () => {
    const id = prompt?.operation_id || operationId;
    if (!id || !reply) return;
    try {
      await hostPromptReply(id, reply);
      setReply("");
      setPrompt(null);
    } catch (cause) {
      failOperation(cause);
    }
  };

  const clearToken = async () => {
    if (!server) return;
    try {
      await clearDaemonToken(server.id);
      setToken("");
      setTokenStored(false);
      onSaved();
    } catch (cause) {
      setFormError(String(cause));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] min-w-0 overflow-x-hidden overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <span>{editing ? "Edit host" : "Add host"}</span>
            {editing && label.trim() && (
              <span className="font-normal text-muted-foreground"> · {label.trim()}</span>
            )}
          </DialogTitle>
          <DialogDescription className="sr-only">
            Configure an SSH or HTTPS connection to a Tariboy host.
          </DialogDescription>
        </DialogHeader>

        {!editing && isDesktop() && (
          <div className="flex gap-2">
            <Button
              type="button"
              variant={transport === "ssh" ? "default" : "outline"}
              size="sm"
              onClick={() => setTransport("ssh")}
            >
              SSH
            </Button>
            <Button
              type="button"
              variant={transport === "https" ? "default" : "outline"}
              size="sm"
              onClick={() => setTransport("https")}
            >
              Advanced HTTPS
            </Button>
          </div>
        )}

        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="add-host-label">{transport === "ssh" ? "Label" : "label"}</Label>
            <Input
              id="add-host-label"
              value={label}
              onChange={(event) => setLabel(event.target.value)}
              className="h-8"
            />
          </div>

          {transport === "ssh" ? (
            <>
              <div className="space-y-1">
                <Label htmlFor="add-host-alias">SSH alias</Label>
                <Input
                  id="add-host-alias"
                  value={sshAlias}
                  onChange={(event) => setSshAlias(event.target.value)}
                  placeholder="the name from ~/.ssh/config"
                  autoComplete="off"
                  className="h-8"
                />
                <p className="text-xs text-muted-foreground">
                  Uses your system OpenSSH config and ssh-agent.
                </p>
              </div>

              <section className="min-w-0 space-y-2 rounded-lg border p-3">
                <h3 className="text-sm font-medium">
                  {operation.kind === "update" ? "Update Tariboy" : "Connect Tariboy"}
                </h3>
                <ol className="space-y-1.5 text-sm">
                  {HOST_PROGRESS_STEPS.map((step) => {
                    const state = stepStates[step.id];
                    const label = operation.kind === "update"
                      ? step.updateLabel
                      : step.provisionLabel;
                    const marker = state === "complete"
                      ? "✓"
                      : state === "failed"
                        ? "×"
                        : state === "active"
                          ? "●"
                          : "○";
                    return (
                      <li
                        key={step.id}
                        data-testid="host-progress-step"
                        data-step={step.id}
                        aria-current={state === "active" ? "step" : undefined}
                        className={
                          state === "failed"
                            ? "flex min-w-0 items-center gap-2 font-medium text-destructive"
                            : state === "active"
                              ? "flex min-w-0 items-center gap-2 font-medium text-foreground"
                              : state === "complete"
                                ? "flex min-w-0 items-center gap-2 text-foreground"
                                : "flex min-w-0 items-center gap-2 text-muted-foreground"
                        }
                      >
                        <span aria-hidden="true" className="w-4 shrink-0 text-center">
                          {marker}
                        </span>
                        <span className="min-w-0">{label}</span>
                        <span className="sr-only"> — {state}</span>
                      </li>
                    );
                  })}
                </ol>

                {operation.status === "succeeded" && (
                  <div className="rounded-md bg-emerald-500/10 p-2 text-sm text-emerald-700 dark:text-emerald-300">
                    <p className="font-medium">
                      {operation.kind === "update" ? "Update complete" : "Host connected"}
                    </p>
                    <p>Tariboy is ready on this host.</p>
                  </div>
                )}

                {operation.status === "failed" && operation.error && (
                  <div
                    role="alert"
                    className="min-w-0 rounded-md bg-destructive/10 p-2 text-sm text-destructive"
                  >
                    <p className="font-medium">
                      {operation.kind === "update" ? "Update failed" : "Connection failed"}
                    </p>
                    <p className="break-words [overflow-wrap:anywhere]">{operation.error}</p>
                  </div>
                )}

                {displayHost?.platform && displayHost.arch && (
                  <p className={unsupported ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
                    {unsupported
                      ? `Unsupported host: ${displayHost.platform}/${displayHost.arch}`
                      : `Server: ${displayHost.platform}/${displayHost.arch}`}
                  </p>
                )}
                {installRequirements.length > 0 && (
                  <div className="space-y-0.5 text-xs text-destructive">
                    <p className="font-medium">Install blocked</p>
                    <p>Missing install requirements: {installRequirements.join(", ")}</p>
                  </div>
                )}
                {optionalTools.length > 0 && (
                  <p className="text-xs text-muted-foreground">
                    Optional agent tools not found: {optionalTools.join(", ")}
                  </p>
                )}

                {output.length > 0 && (
                  <details className="min-w-0 rounded-md border bg-muted/40">
                    <summary className="cursor-pointer px-2 py-1.5 text-xs font-medium">
                      Technical details
                    </summary>
                    <pre
                      aria-label="SSH technical details"
                      className="max-h-40 min-w-0 max-w-full overflow-auto border-t p-2 text-xs whitespace-pre-wrap [overflow-wrap:anywhere]"
                    >
                      {output.map((event) => event.text).join("\n")}
                    </pre>
                  </details>
                )}
              </section>

              {prompt && (
                <div className="space-y-2 rounded border p-2">
                  <div className="text-sm">{prompt.text || "SSH authentication reply required"}</div>
                  <Label htmlFor="host-prompt-reply">SSH reply</Label>
                  <div className="flex gap-2">
                    <Input
                      id="host-prompt-reply"
                      type="password"
                      value={reply}
                      onChange={(event) => setReply(event.target.value)}
                      autoComplete="one-time-code"
                    />
                    <Button type="button" onClick={() => void sendReply()}>
                      Send reply
                    </Button>
                  </div>
                </div>
              )}

            </>
          ) : (
            <>
              <div className="space-y-1">
                <Label htmlFor="add-host-url">base URL</Label>
                <Input
                  id="add-host-url"
                  value={baseURL}
                  onChange={(event) => setBaseURL(event.target.value)}
                  placeholder="https://host:port"
                  className="h-8"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="add-host-token">token</Label>
                <Input
                  id="add-host-token"
                  type="password"
                  value={token}
                  onChange={(event) => setToken(event.target.value)}
                  placeholder={editing ? "leave empty to keep the current token" : ""}
                  className="h-8"
                />
                {editing && (
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground">
                      {tokenStored ? "token set for this session" : "no token set"}
                    </span>
                    {tokenStored && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 px-2 text-xs"
                        onClick={() => void clearToken()}
                      >
                        Clear token
                      </Button>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
          {formError && <p className="text-sm text-destructive">{formError}</p>}
        </div>

        <DialogFooter>
          {transport === "ssh" ? (
            <>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Close
              </Button>
              {operation.status === "running" ? (
                <Button type="button" disabled>
                  {operation.kind === "update" ? "Updating…" : "Connecting…"}
                </Button>
              ) : operation.status === "failed" && operation.kind === "update" ? (
                <Button type="button" onClick={() => void updateHost()}>
                  Retry update
                </Button>
              ) : !editing ? (
                <Button type="button" onClick={() => void submitSsh()}>
                  Add and connect
                </Button>
              ) : dirtySsh || server?.state !== "ready" ? (
                <Button type="button" onClick={() => void submitSsh()}>
                  Save and reconnect
                </Button>
              ) : (
                <Button type="button" onClick={() => void updateHost()}>
                  Update Tariboy
                </Button>
              )}
            </>
          ) : (
            <Button type="button" onClick={() => void submitHttps()}>
              {editing ? "Save" : "Add"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
