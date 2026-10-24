import { useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import { ApiError, type ApiClient, type InboxItem } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import type { DialogMode, DialogState } from "./agentMessagesModel";
import { dialogCopy } from "./agentMessagesModel";

type SubmitArgs = { api: ApiClient; name: string; id: string; text: string };

// The API call behind each dialog mode; each resolves to its success toast.
const SUBMIT: Record<DialogMode, (a: SubmitArgs) => Promise<string>> = {
  clear: async ({ api, name }) => {
    const result = await api.messages.agentInboxClear(name);
    return `deleted ${result.deleted_deliveries} deliveries and ${result.deleted_messages} messages`;
  },
  processed: async ({ api, name, id, text }) => {
    await api.messages.agentInboxProcessed(name, id, text);
    return "marked processed";
  },
  reply: async ({ api, name, id, text }) => {
    await api.messages.agentInboxReply(name, id, text);
    return "replied";
  },
};

// The Mark-processed / Reply / Clear-queue dialog state and its submit, which
// always reloads the active view afterwards.
export function useMessageDialog(reload: () => Promise<void>) {
  const api = useApi();
  const name = useAgentName();
  const [dialog, setDialog] = useState<DialogState>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);

  const openItem = (mode: "processed" | "reply", item: InboxItem) => {
    setText("");
    setDialog({ mode, item });
  };
  const openClear = () => setDialog({ mode: "clear" });
  const close = () => setDialog(null);

  const submit = async () => {
    if (!dialog) return;
    const trimmed = text.trim();
    const required = dialogCopy(dialog.mode).required;
    if (required && !trimmed) {
      toast.error(required);
      return;
    }
    setBusy(true);
    try {
      const id = dialog.mode === "clear" ? "" : dialog.item.id;
      toast.success(await SUBMIT[dialog.mode]({ api, name, id, text: trimmed }));
      setDialog(null);
    } catch (e) {
      toast.error(`failed: ${e instanceof ApiError ? e.message : String(e)}`);
    } finally {
      await reload();
      setBusy(false);
    }
  };

  return { dialog, text, setText, busy, openItem, openClear, close, submit };
}

export type MessageDialogState = ReturnType<typeof useMessageDialog>;
