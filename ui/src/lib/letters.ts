import type { Letter } from "./api";

// Kept apart from lib/api so the shell's eager modules read a letter without
// pulling the API client into the entry chunk.

/** The task a letter is about: the string `data.task_key`, or "". */
export function letterTaskKey(letter: Letter): string {
  const key = letter.data?.task_key;
  return typeof key === "string" ? key : "";
}
