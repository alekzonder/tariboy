import { describe, expect, it } from "vitest";
import { sidebarRoute } from "./appChrome";

describe("sidebarRoute", () => {
  it.each([
    ["/", true],
    ["/tasks", true],
    ["/inbox", true],
    ["/activity", true],
    ["/agents/local/a/console", true],
    ["/servers/local/images", true],
    ["/library/images", true],
    ["/library/sources/tariboy", true],
    ["/settings/hosts", true],
    ["/settings/maintenance/operations/3", true],
    ["/settingsx", false],
    ["/app-settings", false],
    ["/agents", false],
    ["/tasksx", false],
  ])("reads %s as %s", (pathname, expected) => {
    expect(sidebarRoute(pathname)).toBe(expected);
  });
});
