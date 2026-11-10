import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

// The scales of src/index.css. Without them twMerge reads `text-ui` as a color
// and drops the real color class next to it, and it does not know that
// `z-overlay` replaces `z-10`. src/lib/utils.test.ts fails when a list here
// and the `@theme` block differ.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: ["caption", "ui", "body", "title", "heading", "display"],
      radius: ["mark", "chip", "control", "card", "panel"],
      shadow: ["raise", "lift"],
    },
    classGroups: {
      z: [{ z: ["sticky", "overlay", "dialog", "alert"] }],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
