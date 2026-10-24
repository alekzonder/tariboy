/** The message of a failed request or a thrown value, as a person reads it. */
export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
