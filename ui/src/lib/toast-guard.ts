import { toast } from "sonner";
import { ApiError } from "@/lib/api";

/**
 * Run an async action, toasting success or failure. Shared by the page modules
 * that used to each carry a local `guard` copy.
 *
 * - `opts.verb` sets the success suffix (`"saved"` by default; pages that
 *   report `"ok"` pass it explicitly).
 * - `opts.after` runs after a successful action (e.g. reload a list).
 * - The action's resolved value is returned so callers can consume it.
 */
export async function guard<T>(
  label: string,
  fn: () => Promise<T>,
  opts?: { verb?: string; after?: () => void },
): Promise<T | undefined> {
  try {
    const r = await fn();
    toast.success(`${label} ${opts?.verb ?? "saved"}`);
    opts?.after?.();
    return r;
  } catch (e) {
    toast.error(`${label} failed: ${e instanceof ApiError ? e.message : String(e)}`);
  }
}
