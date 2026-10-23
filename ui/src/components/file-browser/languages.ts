import { javascript } from "@codemirror/lang-javascript";
import { python } from "@codemirror/lang-python";
import { json } from "@codemirror/lang-json";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { markdown } from "@codemirror/lang-markdown";
import { go } from "@codemirror/lang-go";
import type { Extension } from "@codemirror/state";
import { fileExt } from "./paths";

// Map a file extension to a CodeMirror language extension. A handful of common
// languages are wired; anything else falls back to a plain (unhighlighted)
// editor.
export function langFor(path: string): Extension[] {
  switch (fileExt(path)) {
    case "js": case "jsx": case "mjs": case "cjs":
      return [javascript({ jsx: true })];
    case "ts":
      return [javascript({ typescript: true })];
    case "tsx":
      return [javascript({ jsx: true, typescript: true })];
    case "py":
      return [python()];
    case "json":
      return [json()];
    case "html": case "htm":
      return [html()];
    case "css":
      return [css()];
    case "md": case "markdown":
      return [markdown()];
    case "go":
      return [go()];
    default:
      return [];
  }
}
