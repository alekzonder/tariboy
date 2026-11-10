import { useEffect, useState } from "react";
import indexCss from "@/index.css?raw";
import tokensCss from "@/styles/tokens.css?raw";

// What the Foundations stories show. The lists are read from the two style
// files, so a token added there appears in Storybook without a second edit.

/** The custom properties that the `:root` block of styles/tokens.css declares, in order. */
export function roles(): string[] {
  const start = tokensCss.indexOf("\n:root {");
  const block = tokensCss.slice(start, tokensCss.indexOf("\n}", start));
  return [...block.matchAll(/^\s*(--[a-z0-9-]+):/gm)].map((match) => match[1]);
}

export type ScaleStep = { name: string; value: string; lineHeight?: string };

/** The steps of a `@theme` scale of index.css: `scale("text")` reads `--text-ui: 12px`. */
export function scale(kind: "text" | "radius" | "z"): ScaleStep[] {
  const pattern = new RegExp(`^\\s*--${kind}-([a-z0-9]+):\\s*([^;]+);`, "gm");
  return [...indexCss.matchAll(pattern)].map((match) => {
    const lineHeight = indexCss.match(
      new RegExp(`--${kind}-${match[1]}--line-height:\\s*([^;]+);`),
    );
    return { name: match[1], value: match[2], lineHeight: lineHeight?.[1] };
  });
}

/** The value a custom property has right now on <html>. It follows the theme switch. */
export function useTokenValue(name: string): string {
  const [value, setValue] = useState("");
  useEffect(() => {
    const read = () =>
      setValue(getComputedStyle(document.documentElement).getPropertyValue(name).trim());
    read();
    const observer = new MutationObserver(read);
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, [name]);
  return value;
}
