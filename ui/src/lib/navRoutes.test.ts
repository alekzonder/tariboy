import { describe, expect, it } from "vitest";
import {
  activeNav,
  legacyServerRedirect,
  legacySettingsPath,
  navPath,
  scopedPath,
} from "./navRoutes";

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
    ["/servers/local/settings", "", "/settings/agent-defaults?host=local"],
    ["/servers/local/settings/", "", "/settings/agent-defaults?host=local"],
    ["/servers/d_abc/settings/hosts", "", "/settings/hosts?host=d_abc"],
    ["/servers/local/settings/advanced/groups", "?team=t1", "/settings/teams?host=local&team=t1"],
    ["/servers/local/settings/operations/7", "", "/settings/maintenance/operations/7?host=local"],
    [
      "/servers/local/settings/integrations/telegram",
      "",
      "/settings/integrations/telegram?host=local",
    ],
    ["/settings", "", "/settings/agent-defaults"],
    ["/settings/", "", "/settings/agent-defaults"],
    ["/settings/advanced/usage", "?range=7d", "/settings/usage?range=7d"],
    ["/settings/advanced/groups", "?team=t1", "/settings/teams?team=t1"],
    ["/settings/operations/3", "", "/settings/maintenance/operations/3"],
    ["/settings/unknown/page", "", "/settings/hosts"],
  ])("sends %s%s to %s", (pathname, search, target) => {
    expect(legacyServerRedirect(pathname, search)).toBe(target);
  });

  it.each([
    ["/", ""],
    ["/tasks", "?host=local"],
    ["/library/images", "?host=local"],
    ["/servers/local/imagesx", ""],
    ["/settings/hosts", ""],
    ["/settings/teams", "?team=t1"],
    ["/settings/usage", "?host=d_abc"],
    ["/settings/agent-defaults", ""],
    ["/settings/integrations", ""],
    ["/settings/integrations/telegram", ""],
    ["/settings/maintenance", ""],
    ["/settings/maintenance/operations/4", ""],
    ["/agents/local/a1/advanced", "?view=logs"],
  ])("leaves %s%s alone", (pathname, search) => {
    expect(legacyServerRedirect(pathname, search)).toBeNull();
  });
});

describe("legacySettingsPath", () => {
  it.each([
    ["", "/settings/agent-defaults"],
    ["hosts", "/settings/hosts"],
    ["advanced/daemons", "/settings/hosts"],
    ["cli", "/settings/hosts"],
    ["appearance", "/settings/hosts"],
    ["advanced", "/settings/hosts"],
    ["advanced/groups", "/settings/teams"],
    ["advanced/usage", "/settings/usage"],
    ["advanced/budgets", "/settings/usage"],
    ["operations", "/settings/maintenance"],
    ["advanced/ops", "/settings/maintenance"],
    ["operations/12", "/settings/maintenance/operations/12"],
    ["advanced/plugins", "/settings/integrations"],
    ["integrations/telegram", "/settings/integrations/telegram"],
    ["teams", "/settings/teams"],
    ["maintenance/operations/2", "/settings/maintenance/operations/2"],
    ["nothing/here", "/settings/hosts"],
  ])("maps the old tail %j to %s", (tail, target) => {
    expect(legacySettingsPath(tail)).toBe(target);
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
    ["settings", "/settings/hosts"],
  ] as const)("opens %s at %s", (item, path) => {
    expect(navPath(item, "d_abc")).toBe(path);
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
    ["/settings/hosts", "settings"],
    ["/settings/maintenance/operations/3", "settings"],
    ["/settingsx", null],
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
