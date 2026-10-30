import { expect, test } from "./fixture";

// The shared text controls set the attributes on each field; the WebView must
// keep them on a field rendered by the app.
test("the real Desktop WebView disables automatic input assistance", async ({ desktop }) => {
  await expect
    .poll(() =>
      desktop.execute<Record<string, string | boolean | null> | null>(`
    if (typeof window.__TAURI_INTERNALS__ !== "object") return null;
    const toggle = document.querySelector('header button[aria-label="Show agents"]');
    if (toggle instanceof HTMLButtonElement) toggle.click();
    const field = document.querySelector('input[aria-label="Search agents"]');
    if (!(field instanceof HTMLInputElement)) return null;
    return {
      spellcheck: field.getAttribute("spellcheck"),
      autocorrect: field.getAttribute("autocorrect"),
      autocapitalize: field.getAttribute("autocapitalize"),
      spellcheckOff: field.spellcheck === false,
    };
  `),
    )
    .toEqual({
      spellcheck: "false",
      autocorrect: "off",
      autocapitalize: "off",
      spellcheckOff: true,
    });
});
