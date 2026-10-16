import { useCallback, useRef, useState } from "react";
import { normalizeField, validateField, type Values } from "./registry";

// A commit unit is one server request. Most hold one field; a budget or a
// retention policy holds several fields that the daemon takes in one request.
export interface CommitUnit {
  id: string;
  keys: readonly string[];
  // submit receives the whole page draft (unchanged fields hold the baseline),
  // already normalized.
  submit: (values: Values) => Promise<unknown>;
}

export interface ConfigDraft {
  values: Values;
  dirtyKeys: string[];
  saving: boolean;
  error: string;
  fieldErrors: Values;
  notice: string;
  setField: (key: string, value: string) => void;
  save: () => Promise<void>;
  reset: () => void;
}

const PARTIAL_FAILURE =
  "Some changes were not saved. Review the highlighted fields and try again.";
const INVALID = "Fix the highlighted fields before saving.";

// useConfigDraft owns the page's one save contract: one draft over every
// block, validation before any request, the changed units sent serially in
// page order, a stop at the first failure, and reconciliation from a reload.
//
// The baseline is not state: it is always the loaded values, so a reload is
// reconciled by construction. Only edited fields are held as drafts, and a
// field drops its draft exactly when the server acknowledges it — so a partial
// failure keeps the failed and unsent drafts while the acknowledged ones adopt
// the server's canonical values. A field whose source has not loaded (no
// baseline key) is never dirty and never sent.
export function useConfigDraft({
  baseline,
  units,
  reload,
  onFocus,
}: {
  baseline: Values;
  units: readonly CommitUnit[];
  reload: () => Promise<unknown>;
  onFocus?: (key: string) => void;
}): ConfigDraft {
  const [drafts, setDrafts] = useState<Values>({});
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Values>({});
  const [notice, setNotice] = useState("");

  const values: Values = { ...baseline };
  for (const [key, raw] of Object.entries(drafts)) {
    if (key in baseline) values[key] = raw;
  }
  const normalized: Values = Object.fromEntries(
    Object.entries(values).map(([key, raw]) => [key, normalizeField(key, raw)]),
  );
  const dirtyKeys = units.flatMap((unit) =>
    unit.keys.filter((key) => key in drafts && key in baseline && normalized[key] !== baseline[key]),
  );

  const setField = useCallback((key: string, value: string) => {
    setNotice("");
    setError("");
    setFieldErrors((prev) => {
      if (!(key in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
    setDrafts((prev) => ({ ...prev, [key]: value }));
  }, []);

  const reset = useCallback(() => {
    setError("");
    setFieldErrors({});
    setNotice("Changes reset");
    setDrafts({});
  }, []);

  const save = async () => {
    if (savingRef.current || dirtyKeys.length === 0) return;
    const invalid: Values = {};
    for (const key of dirtyKeys) {
      const message = validateField(key, normalized[key], normalized);
      if (message) invalid[key] = message;
    }
    // A pair such as a secret key and its value is checked as a pair: an
    // unchanged partner can be the one that is missing.
    for (const unit of units) {
      if (!unit.keys.some((key) => dirtyKeys.includes(key))) continue;
      for (const key of unit.keys) {
        const message = key in baseline ? validateField(key, normalized[key], normalized) : "";
        if (message) invalid[key] = message;
      }
    }
    const firstInvalid = Object.keys(invalid)[0];
    if (firstInvalid) {
      setError(INVALID);
      setFieldErrors(invalid);
      setNotice("");
      onFocus?.(firstInvalid);
      return;
    }
    savingRef.current = true;
    setSaving(true);
    setError("");
    setFieldErrors({});
    setNotice("");
    const sent: Values = {};
    let failedKey = "";
    let failedMessage = "";
    for (const unit of units) {
      const changed = unit.keys.filter((key) => dirtyKeys.includes(key));
      if (changed.length === 0) continue;
      try {
        await unit.submit(normalized);
        for (const key of unit.keys) if (key in drafts) sent[key] = drafts[key];
      } catch (cause) {
        // Stop here: every unit after this one stays an unsent dirty draft.
        failedKey = changed[0];
        failedMessage = cause instanceof Error ? cause.message : String(cause);
        break;
      }
    }
    // Reload first, then release the acknowledged drafts, so those fields move
    // straight from the operator's value to the server's canonical one. A
    // field edited again while the save ran keeps its newer draft.
    await reload();
    setDrafts((prev) => {
      const next = { ...prev };
      for (const [key, value] of Object.entries(sent)) {
        if (next[key] === value) delete next[key];
      }
      return next;
    });
    if (failedKey) {
      setError(PARTIAL_FAILURE);
      setFieldErrors({ [failedKey]: failedMessage });
      onFocus?.(failedKey);
    } else {
      setNotice("All changes saved");
    }
    savingRef.current = false;
    setSaving(false);
  };

  return { values, dirtyKeys, saving, error, fieldErrors, notice, setField, save, reset };
}
