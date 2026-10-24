import { describe, expect, it } from "vitest";
import { emptyScriptForm, scriptRequest } from "./scriptRequest";

const base = {
  ...emptyScriptForm,
  script_name: "nightly",
  description: "backup database",
  command: "backup",
};
const spec = { script_name: "nightly", description: "backup database", command: "backup" };

describe("scriptRequest", () => {
  it("builds a one-shot request and ignores the interval", () => {
    expect(scriptRequest({ ...base, mode: "once", interval_seconds: "abc" })).toEqual({
      mode: "once",
      spec,
    });
  });

  it("builds a scheduled request with a numeric interval", () => {
    expect(scriptRequest({ ...base, mode: "every", interval_seconds: "60" })).toEqual({
      mode: "every",
      spec: { ...spec, interval_seconds: 60 },
    });
  });

  it.each(["", "0", "-5", "1.5", "abc"])("refuses the scheduled interval %j", (interval) => {
    expect(scriptRequest({ ...base, mode: "every", interval_seconds: interval })).toEqual({
      error: "Scheduled scripts need a positive interval in seconds",
    });
  });
});
