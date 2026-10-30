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
    ["/servers/prod/images", "", "/library/images?host=prod"],
    ["/servers/prod/images/", "", "/library/images?host=prod"],
    ["/servers/prod/images/reviewer", "", "/library/images/reviewer?host=prod"],
    [
      "/servers/prod/images/reviewer/v3/template",
      "?x=1",
      "/library/images/reviewer/v3/template?host=prod&x=1",
    ],
    ["/servers/local/images/a%2Fb/v%201", "", "/library/images/a%2Fb/v%201?host=local"],
    ["/servers/local/workflows", "", "/library/workflows?host=local"],
    ["/servers/local/workflows/flow/latest", "", "/library/workflows/flow/latest?host=local"],
    ["/servers/local/stores", "", "/library/sources?host=local"],
    ["/servers/local/stores/tariboy", "", "/library/sources/tariboy?host=local"],
    ["/servers/remote%201/stores", "?host=x", "/library/sources?host=remote+1"],
  ])("sends %s%s to %s", (pathname, search, target) => {
    expect(legacyServerRedirect(pathname, search)).toBe(target);
  });

  it.each([
    ["/", ""],
    ["/tasks", "?host=local"],
    ["/library/images", "?host=local"],
    ["/servers/local/imagesx", ""],
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
    ["library-images", "/library/images"],
    ["library-workflows", "/library/workflows"],
    ["library-sources", "/library/sources"],
    ["settings", "/servers/d_abc/settings"],
  ] as const)("opens %s at %s", (item, path) => {
    expect(navPath(item, "d_abc")).toBe(path);
  });

  it("names the local host in Settings paths", () => {
    expect(navPath("settings", "")).toBe("/servers/local/settings");
  });

  it.each([
    ["/", "home"],
    ["/activity", "home"],
    ["/tasks", "tasks"],
    ["/inbox", "inbox"],
    ["/library/images", "library"],
    ["/library/images/basic/latest/skills", "library"],
    ["/library/workflows/x/1", "library"],
    ["/library/sources/tariboy", "library"],
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
