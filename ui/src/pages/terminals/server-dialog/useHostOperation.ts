import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { DaemonMeta } from "@/lib/daemons";
import {
  hostConnected,
  onHostState,
  onHostProvisionOutput,
  type DesktopHostView,
  type HostOutputEvent,
} from "@/lib/desktop";
import {
  formatHostOperationError,
  hostStepForPhase,
  isTerminalHostOutput,
  type HostOperationKind,
} from "@/lib/hostProgress";
import { hostMeta, idleOperation, preflightView, type HostOperation } from "./hostOperation";

export function useHostOperation(
  open: boolean,
  transport: "ssh" | "https",
  server: DaemonMeta | undefined,
  onSaved: () => void,
) {
  const hostIdRef = useRef(server?.id ?? "");
  const [host, setHost] = useState<DesktopHostView | null>(null);
  const [operationId, setOperationId] = useState("");
  const operationIdRef = useRef("");
  const initialOperation = idleOperation(
    server?.kind === "ssh" && hostConnected(server.state) ? "update" : "provision",
  );
  const [operation, setOperation] = useState<HostOperation>(initialOperation);
  const operationRef = useRef<HostOperation>(initialOperation);
  const [output, setOutput] = useState<HostOutputEvent[]>([]);
  const [prompt, setPrompt] = useState<HostOutputEvent | null>(null);
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
      const existingUnavailable = operationRef.current.kind === "connect"
        && next.state === "connecting"
        && next.message.startsWith("remote daemon health timed out:");
      const existingNeedsAuth = operationRef.current.kind === "connect"
        && next.state === "needs_auth";
      if (
        (next.state === "failed" || existingNeedsAuth || existingUnavailable)
        && operationRef.current.status !== "idle"
      ) {
        updateOperation((current) => ({
          ...current,
          status: "failed",
          currentStep: existingUnavailable
            ? "reconnect"
            : current.kind === "connect"
              ? "connect"
              : phaseStep ?? current.currentStep ?? "connect",
          error: existingUnavailable
            ? "Existing Tariboy is unavailable at 127.0.0.1:9990."
            : formatHostOperationError(next.message),
        }));
      } else if (
        hostConnected(next.state)
        && (
          operationRef.current.status === "running"
          || (operationRef.current.kind === "connect" && operationRef.current.status === "failed")
        )
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

  const beginOperation = (kind: HostOperationKind) => {
    operationIdRef.current = "";
    setOperationId("");
    setOutput([]);
    setPrompt(null);
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

  const resetOperation = useCallback(() => {
    hostIdRef.current = server?.id ?? "";
    setHost(null);
    setOperationId("");
    operationIdRef.current = "";
    const nextOperation = idleOperation(
      server?.kind === "ssh" && hostConnected(server.state)
        ? "update"
        : "provision",
    );
    operationRef.current = nextOperation;
    setOperation(nextOperation);
    setOutput([]);
    setPrompt(null);
  }, [server]);
  return {
    hostIdRef,
    host,
    setHost,
    operationId,
    operation,
    output,
    prompt,
    setPrompt,
    updateOperation,
    displayHost,
    beginOperation,
    rememberOperationId,
    failOperation,
    resetOperation,
  };
}
