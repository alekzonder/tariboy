import { describe, expect, it } from "vitest";
import type { AgentImageStatus } from "@/lib/api";
import type { AgentView } from "@/lib/types";
import { readBaseline, viewBaseline } from "./commits";

const view = {
  cwd: "/work",
  harness: "claude",
  interactive: true,
  interval_s: 30,
  on_timeout: "",
  goal_enabled: false,
} as unknown as AgentView;

describe("viewBaseline", () => {
  it("reads the view fields as text with their defaults", () => {
    const values = viewBaseline(view);
    expect(values).toMatchObject({
      cwd: "/work",
      "cwd-create": "false",
      harness: "claude",
      model: "",
      interactive: "true",
      interval: "30",
      "on-timeout": "restart",
      "on-error": "restart",
      "max-idle": "0",
      "ai-stall-timeout": "1800",
      "goal-enabled": "false",
    });
    expect(values).not.toHaveProperty("budget-hour");
  });

  it("adds the budget keys only when the view projects a budget", () => {
    const values = viewBaseline({
      ...view,
      budget: { hour_usd: 1, day_usd: 2, week_usd: 3, month_usd: 4 },
    } as unknown as AgentView);
    expect(values).toMatchObject({
      "budget-hour": "1",
      "budget-day": "2",
      "budget-week": "3",
      "budget-month": "4",
    });
  });
});

describe("readBaseline", () => {
  const none = { view: null, imageStatus: null, script: null, retention: null, secrets: null };

  it("contributes no keys for sources that have not loaded", () => {
    expect(readBaseline(none)).toEqual({});
  });

  it("prefers the pending image over the current one", () => {
    const imageStatus = {
      current: { ref: "base:1", digest: "" },
      pending: { ref: "", digest: "", error: "" },
    } as AgentImageStatus;
    expect(readBaseline({ ...none, imageStatus })).toEqual({ image: "base:1" });
    expect(
      readBaseline({
        ...none,
        imageStatus: { ...imageStatus, pending: { ref: "base:2", digest: "", error: "" } },
      }),
    ).toEqual({ image: "base:2" });
  });

  it("keeps an empty script verbatim and reads retention and secrets", () => {
    expect(readBaseline({ ...none, script: "" })).toEqual({ "shell-script": "" });
    expect(readBaseline({ ...none, retention: { override: { keep_days: 7 } } } as never)).toEqual({
      "keep-iterations": "0",
      "keep-days": "7",
    });
    expect(readBaseline({ ...none, secrets: [] })).toEqual({
      "secrets-remove": "",
      "secret-key": "",
      "secret-value": "",
    });
  });

  it("normalizes the values it reads", () => {
    expect(readBaseline({ ...none, view: { ...view, cwd: " /work " } as AgentView }).cwd).toBe(
      "/work",
    );
  });
});
