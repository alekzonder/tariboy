import { addDaemon, clearDaemonToken, setDaemonToken, updateDaemon } from "@/lib/daemons";
import {
  hostConnect,
  hostPromptReply,
  hostProvision,
  hostSaveSsh,
  hostUpdate,
  isAndroidShell,
} from "@/lib/desktop";
import type { HostOperationKind } from "@/lib/hostProgress";
import type { ServerDialogProps } from "../ServerDialog";
import { useHostOperation } from "./useHostOperation";
import { useServerFields, useServerFormReset } from "./useServerFields";
import { validateHttps, validateSsh } from "./serverFormValidation";

export function useServerForm({ open, server, onOpenChange, onSaved }: ServerDialogProps) {
  const editing = server !== undefined;
  const { resetFields, setTokenStored, ...fields } = useServerFields(server);
  const { transport, label, sshAlias, sshSetup, setSshSetup, baseURL, token, setToken } = fields;
  const { reply, setReply, setFormError } = fields;
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
  useServerFormReset(open, server, resetFields, resetOperation, setFormError);

  const dirtySsh =
    editing &&
    (label.trim() !== (server?.label ?? "") || sshAlias.trim() !== (server?.sshAlias ?? ""));
  const submitHttps = async () => {
    const cleanLabel = label.trim();
    const cleanURL = baseURL.trim();
    const cleanToken = token.trim();
    const invalid = validateHttps(cleanLabel, cleanURL, isAndroidShell());
    if (invalid) return setFormError(invalid);
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
    const invalid = validateSsh(cleanLabel, cleanAlias);
    if (invalid) return setFormError(invalid);
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
    ...fields,
    editing,
    dirtySsh,
    submitHttps,
    submitSsh,
    updateHost,
    sendReply,
    clearToken,
  };
}

export type ServerForm = ReturnType<typeof useServerForm>;
