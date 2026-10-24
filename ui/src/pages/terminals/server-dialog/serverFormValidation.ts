import { androidBaseURLError } from "@/lib/desktop";
import type { HostOperation } from "./hostOperation";

/** The first problem with an HTTPS host form, or null when it can be saved. */
export function validateHttps(label: string, url: string, android: boolean): string | null {
  if (!label) return "label is required";
  if (android) return androidBaseURLError(url) || null;
  if (!url || !url.startsWith("http")) return "base URL must start with http";
  return null;
}

/** The first problem with an SSH host form, or null when it can be saved. */
export function validateSsh(label: string, alias: string): string | null {
  if (!label) return "label is required";
  if (!alias) return "SSH alias is required";
  return null;
}

export type SshFooterAction =
  | "running"
  | "retry-update"
  | "retry-connect"
  | "add"
  | "reconnect"
  | "update";

/** Which primary action the SSH form's footer offers. */
export function sshFooterAction(
  operation: HostOperation,
  editing: boolean,
  dirtySsh: boolean,
  connected: boolean,
): SshFooterAction {
  if (operation.status === "running") return "running";
  if (operation.status === "failed" && operation.kind === "update") return "retry-update";
  if (operation.status === "failed" && operation.kind === "connect") return "retry-connect";
  if (!editing) return "add";
  return dirtySsh || !connected ? "reconnect" : "update";
}
