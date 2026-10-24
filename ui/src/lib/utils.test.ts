import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("treats the named text sizes as font sizes, not colors", () => {
    expect(cn("text-muted-foreground", "text-ui")).toBe("text-muted-foreground text-ui");
    expect(cn("text-ui", "text-meta")).toBe("text-meta");
    expect(cn("text-[12px]", "text-body-sm")).toBe("text-body-sm");
  });

  it("knows every text size and radius that index.css declares", () => {
    const css = readFileSync(resolve(__dirname, "../index.css"), "utf8");
    const names = (kind: string) =>
      [...css.matchAll(new RegExp(`^\\s*--${kind}-([a-z0-9-]+):`, "gm"))].map((m) => m[1]);
    expect(names("text")).not.toHaveLength(0);
    for (const name of names("text"))
      expect(cn("text-muted-foreground", `text-${name}`)).toBe(
        `text-muted-foreground text-${name}`,
      );
    for (const name of names("radius"))
      expect(cn("rounded-full", `rounded-${name}`)).toBe(`rounded-${name}`);
  });

  it("lets a named radius replace another radius", () => {
    expect(cn("rounded-md", "rounded-chip")).toBe("rounded-chip");
    expect(cn("rounded-control", "rounded-full")).toBe("rounded-full");
  });
});
