/** A key that lets the daemon recognize a retried write. The prefix names the fallback key when the WebView has no randomUUID. */
export function idempotencyKey(prefix: string): string {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}-${Date.now()}-${Math.random()}`;
}
