import { ApiError } from "@/lib/api";
import { errorText } from "@/lib/errors";

/** The daemon does not show the task to this reader, or it is gone. */
export const isHiddenTaskError = (error: unknown) =>
  error instanceof ApiError && (error.status === 404 || error.status === 403);

/** Why the panel cannot show `key`. A task opened from a link in the panel
 *  that the reader cannot see gets one sentence instead of the daemon's. */
export function taskReadErrorText(key: string, error: unknown, linked: boolean): string {
  if (linked && isHiddenTaskError(error)) {
    return `You cannot open ${key}. It is hidden from you or no longer exists.`;
  }
  return errorText(error);
}
