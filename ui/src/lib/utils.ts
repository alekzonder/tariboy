import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

// The named text sizes and radii of src/index.css. Without them twMerge reads
// `text-ui` as a color and drops the real color class next to it.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: [
        "nano",
        "micro",
        "tiny",
        "caption",
        "meta",
        "ui",
        "body-sm",
        "body",
        "body-lg",
        "title",
      ],
      radius: ["hair", "chip", "control", "panel", "card"],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
