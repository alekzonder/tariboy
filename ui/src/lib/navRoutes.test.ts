import { describe, expect, it } from "vitest";
import { activeNav, legacyServerRedirect, navPath, scopedPath } from "./navRoutes";

describe("legacyServerRedirect", () => {
  it.each([
    ["/servers/local/mail", "", "/inbox?host=local"],
    ["/servers/local/tasks", "", "/tasks?host=local"],
    ["/servers/local/activity", "", "/activity?host=local"],
    ["/servers/d_abc/tasks", "?task=DEV-1", "/tasks?host=d_abc&task=DEV-1"],
    ["/servers/remote%201/mail", "", "/inbox?host=remote+1"],
    [
      "/servers/d_abc/activity",
      "?view=agents&period=7d",
      "/activity?host=d_abc&view=agents&period=7d",
    ],
    ["/servers/d_abc/tasks/", "", "/tasks?host=d_abc"],
    ["/", "?view=all", "/tasks"],
    ["/", "?view=all&task=DEV-2", "/tasks?task=DEV-2"],
    ["/agents/local/a1/console", "?view=all", "/tasks"],
  ])("sends %s%s to %s", (pathname, search, target) => {
    expect(legacyServerRedirect(pathname, search)).toBe(target);
  });

  it.each([
    ["/", ""],
    ["/tasks", "?host=local"],
    ["/servers/local/images", ""],
    ["/servers/local/settings/hosts", ""],
    ["/agents/local/a1/advanced", "?view=logs"],
  ])("leaves %s%s alone", (pathname, search) => {
    expect(legacyServerRedirect(pathname, search)).toBeNull();
  });
});

describe("navPath and activeNav", () => {
  it.each([
    ["home", "/"],
    ["tasks", "/tasks"],
    ["inbox", "/inbox"],
    ["library-images", "/servers/d_abc/images"],
    ["library-workflows", "/servers/d_abc/workflows"],
    ["library-sources", "/servers/d_abc/stores"],
    ["settings", "/servers/d_abc/settings"],
  ] as const)("opens %s at %s", (item, path) => {
    expect(navPath(item, "d_abc")).toBe(path);
  });

  it("names the local host in Library and Settings paths", () => {
    expect(navPath("library-images", "")).toBe("/servers/local/images");
  });

  it.each([
    ["/", "home"],
    ["/activity", "home"],
    ["/tasks", "tasks"],
    ["/inbox", "inbox"],
    ["/servers/local/images", "library"],
    ["/servers/local/images/basic/latest/skills", "library"],
    ["/servers/d1/workflows/x/1", "library"],
    ["/servers/d1/stores/tariboy", "library"],
    ["/servers/local/settings", "settings"],
    ["/servers/local/settings/advanced/usage", "settings"],
    ["/app-settings", "settings"],
    ["/agents/local/a1/console", null],
    ["/agents/local/teams/t", null],
  ])("marks %s as %s", (pathname, item) => {
    expect(activeNav(pathname)).toBe(item);
  });
});

describe("scopedPath", () => {
  it("names the host only when given", () => {
    expect(scopedPath("tasks")).toBe("/tasks");
    expect(scopedPath("tasks", "")).toBe("/tasks?host=local");
    expect(scopedPath("inbox", "d_abc", { letter: "m-1" })).toBe("/inbox?host=d_abc&letter=m-1");
    expect(scopedPath("tasks", undefined, { task: "DEV-1" })).toBe("/tasks?task=DEV-1");
  });
});
