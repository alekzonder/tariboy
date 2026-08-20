import { expect, test } from "./fixture";

test("the real Desktop WebView disables automatic input assistance", async ({ desktop }) => {
  await expect.poll(() => desktop.execute<Record<string, string | number | boolean | null> | null>(`
    const root = document.getElementById("root");
    if (typeof window.__TAURI_INTERNALS__ !== "object" || !root) return null;
    const input = document.createElement("input");
    const textarea = document.createElement("textarea");
    root.append(input, textarea);
    const attributes = {
      spellcheck: root.getAttribute("spellcheck"),
      autocorrect: root.getAttribute("autocorrect"),
      autocapitalize: root.getAttribute("autocapitalize"),
      editableCount: 2,
      editablesDisableSpellcheck: input.spellcheck === false && textarea.spellcheck === false,
    };
    input.remove();
    textarea.remove();
    return attributes;
  `)).toEqual({
    spellcheck: "false",
    autocorrect: "off",
    autocapitalize: "off",
    editableCount: 2,
    editablesDisableSpellcheck: true,
  });
});
