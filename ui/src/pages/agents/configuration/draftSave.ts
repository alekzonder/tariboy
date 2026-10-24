// The pure half of the page's Save: which fields block it, which requests it
// sends, and which drafts the server acknowledged. useConfigDraft owns the
// state around these steps.
import { validateField, type Values } from "./registry";
import type { CommitUnit } from "./useConfigDraft";

// validateDraft returns a message for every field that blocks a Save. A pair
// such as a secret key and its value is checked as a pair: an unchanged
// partner can be the one that is missing.
export function validateDraft({
  dirtyKeys,
  units,
  baseline,
  normalized,
}: {
  dirtyKeys: readonly string[];
  units: readonly CommitUnit[];
  baseline: Values;
  normalized: Values;
}): Values {
  const invalid: Values = {};
  for (const key of dirtyKeys) {
    const message = validateField(key, normalized[key], normalized);
    if (message) invalid[key] = message;
  }
  for (const unit of units) {
    if (!unit.keys.some((key) => dirtyKeys.includes(key))) continue;
    for (const key of unit.keys) {
      const message = key in baseline ? validateField(key, normalized[key], normalized) : "";
      if (message) invalid[key] = message;
    }
  }
  return invalid;
}

// submitUnits sends the changed units serially in page order and stops at the
// first failure: every unit after it stays an unsent dirty draft. sent holds
// the drafts of the acknowledged units.
export async function submitUnits({
  units,
  dirtyKeys,
  drafts,
  normalized,
}: {
  units: readonly CommitUnit[];
  dirtyKeys: readonly string[];
  drafts: Values;
  normalized: Values;
}): Promise<{ sent: Values; failedKey: string; failedMessage: string }> {
  const sent: Values = {};
  for (const unit of units) {
    const changed = unit.keys.filter((key) => dirtyKeys.includes(key));
    if (changed.length === 0) continue;
    try {
      await unit.submit(normalized);
      for (const key of unit.keys) if (key in drafts) sent[key] = drafts[key];
    } catch (cause) {
      const failedMessage = cause instanceof Error ? cause.message : String(cause);
      return { sent, failedKey: changed[0], failedMessage };
    }
  }
  return { sent, failedKey: "", failedMessage: "" };
}

// releaseSent drops the acknowledged drafts. A field edited again while the
// save ran keeps its newer draft.
export function releaseSent(drafts: Values, sent: Values): Values {
  const next = { ...drafts };
  for (const [key, value] of Object.entries(sent)) {
    if (next[key] === value) delete next[key];
  }
  return next;
}
