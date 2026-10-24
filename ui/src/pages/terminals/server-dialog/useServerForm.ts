import { useEffect, useState } from "react";
import {
  addDaemon,
  clearDaemonToken,
  hasDaemonToken,
  setDaemonToken,
  updateDaemon,
} from "@/lib/daemons";
import {
  androidBaseURLError,
  hostConnect,
  hostPromptReply,
  hostProvision,
  hostSaveSsh,
  hostUpdate,
  isAndroidShell,
  isDesktop,
} from "@/lib/desktop";
import type { HostOperationKind } from "@/lib/hostProgress";
import type { ServerDialogProps } from "../ServerDialog";
import { useHostOperation } from "./useHostOperation";

export function useServerForm({ open, server, onOpenChange, onSaved }: ServerDialogProps) {
  const editing = server !== undefined;
  const [transport, setTransport] = useState<"ssh" | "https">(
    server?.kind ?? (isDesktop() ? "ssh" : "https"),
  );
  const [label, setLabel] = useState(server?.label ?? "");
  const [sshAlias, setSshAlias] = useState(server?.sshAlias ?? "");
  const [sshSetup, setSshSetup] = useState<"install" | "connect">("install");
  const [baseURL, setBaseURL] = useState(server?.baseURL ?? "");
  const [token, setToken] = useState("");
  const [tokenStored, setTokenStored] = useState(false);
  const [reply, setReply] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const hostOperation = useHostOperation(open, transport, server, onSaved);
  const {
    hostIdRef,
    setHost,
    operationId,
    prompt,
    setPrompt,
    rememberOperationId,
    failOperation,
    resetOperation,
  } = hostOperation;
  const beginOperation = (kind: HostOperationKind) => {
    hostOperation.beginOperation(kind);
    setReply("");
    setFormError(null);
  };
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
        setSshSetup("install");
        setBaseURL(server?.baseURL ?? "");
        setToken("");
        setTokenStored(stored);
        resetOperation();
        setReply("");
        setFormError(null);
      } catch (cause) {
        if (!cancelled) setFormError(String(cause));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, server, resetOperation]);

  const dirtySsh =
    editing &&
    (label.trim() !== (server?.label ?? "") || sshAlias.trim() !== (server?.sshAlias ?? ""));
  const submitHttps = async () => {
    const cleanLabel = label.trim();
    const cleanURL = baseURL.trim();
    const cleanToken = token.trim();
    if (!cleanLabel) return setFormError("label is required");
    if (isAndroidShell()) {
      const urlError = androidBaseURLError(cleanURL);
      if (urlError) return setFormError(urlError);
    } else if (!cleanURL || !cleanURL.startsWith("http")) {
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

  const submitSsh = async (setup = sshSetup) => {
    const cleanLabel = label.trim();
    const cleanAlias = sshAlias.trim();
    if (!cleanLabel) return setFormError("label is required");
    if (!cleanAlias) return setFormError("SSH alias is required");
    setSshSetup(setup);
    beginOperation(setup === "connect" ? "connect" : "provision");
    try {
      const saved = await hostSaveSsh({
        id: hostIdRef.current || server?.id,
        label: cleanLabel,
        ssh_alias: cleanAlias,
      });
      if (!saved) throw new Error("host_save_ssh is unavailable outside the desktop shell");
      hostIdRef.current = saved.id;
      setHost(saved);
      onSaved();
      if (setup === "connect") {
        await hostConnect(saved.id);
      } else {
        const operation = await hostProvision(saved.id);
        if (!operation) throw new Error("host_provision is unavailable outside the desktop shell");
        rememberOperationId(operation.operation_id);
      }
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

  return {
    ...hostOperation,
    editing,
    transport,
    setTransport,
    label,
    setLabel,
    sshAlias,
    setSshAlias,
    sshSetup,
    setSshSetup,
    baseURL,
    setBaseURL,
    token,
    setToken,
    tokenStored,
    reply,
    setReply,
    formError,
    dirtySsh,
    submitHttps,
    submitSsh,
    updateHost,
    sendReply,
    clearToken,
  };
}

export type ServerForm = ReturnType<typeof useServerForm>;
