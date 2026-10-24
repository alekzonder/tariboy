// validChannel mirrors the daemon's bus.ValidChannel (internal/bus/bus.go): a
// known kind prefix plus every ':'-separated segment matching the name rule. It
// gates free-text input client-side so a typo never POSTs a junk channel — the
// daemon still re-validates, this just surfaces the error inline before submit.
const CHANNEL_SEG = /^[a-z0-9][a-z0-9_-]*$/;
const CHANNEL_PREFIXES = new Set(["agent", "group", "user", "chat", "plugin", "system"]);
export function validChannel(name: string): boolean {
  if (!name || !name.includes(":")) return false;
  const parts = name.split(":");
  if (!CHANNEL_PREFIXES.has(parts[0])) return false;
  return parts.every((p) => CHANNEL_SEG.test(p));
}
