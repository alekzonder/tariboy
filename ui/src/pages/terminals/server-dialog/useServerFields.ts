import { useCallback, useEffect, useState } from "react";
import { hasDaemonToken, type DaemonMeta } from "@/lib/daemons";
import { isDesktop } from "@/lib/desktop";

const transportFor = (server?: DaemonMeta) => server?.kind ?? (isDesktop() ? "ssh" : "https");

/** The host form's own fields, and the one way to put them back to `server`. */
export function useServerFields(server: DaemonMeta | undefined) {
  const [transport, setTransport] = useState<"ssh" | "https">(transportFor(server));
  const [label, setLabel] = useState(server?.label ?? "");
  const [sshAlias, setSshAlias] = useState(server?.sshAlias ?? "");
  const [sshSetup, setSshSetup] = useState<"install" | "connect">("install");
  const [baseURL, setBaseURL] = useState(server?.baseURL ?? "");
  const [token, setToken] = useState("");
  const [tokenStored, setTokenStored] = useState(false);
  const [reply, setReply] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const resetFields = useCallback((next: DaemonMeta | undefined, stored: boolean) => {
    setTransport(transportFor(next));
    setLabel(next?.label ?? "");
    setSshAlias(next?.sshAlias ?? "");
    setSshSetup("install");
    setBaseURL(next?.baseURL ?? "");
    setToken("");
    setTokenStored(stored);
    setReply("");
    setFormError(null);
  }, []);

  return {
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
    setTokenStored,
    reply,
    setReply,
    formError,
    setFormError,
    resetFields,
  };
}

/** Every time the dialog opens, start the form over from `server`, reading
 *  whether an HTTPS host already has a stored token. */
export function useServerFormReset(
  open: boolean,
  server: DaemonMeta | undefined,
  resetFields: (server: DaemonMeta | undefined, stored: boolean) => void,
  resetOperation: () => void,
  setFormError: (error: string) => void,
) {
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    void (async () => {
      try {
        const stored =
          server && transportFor(server) === "https" ? await hasDaemonToken(server.id) : false;
        if (cancelled) return;
        resetFields(server, stored);
        resetOperation();
      } catch (cause) {
        if (!cancelled) setFormError(String(cause));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, server, resetFields, resetOperation, setFormError]);
}
