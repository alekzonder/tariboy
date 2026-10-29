import { describe, expect, it } from "vitest";
import changelog from "../../../../CHANGELOG.md?raw";
import { parseChangelog } from "./changelog";
import { UI_VERSION } from "./version";

const SAMPLE = `## [0.2.0] - 2026-02-01

### Added

- Two.

[0.2.0]: https://example.com/compare/v0.1.0...v0.2.0

## [0.1.0] - 2026-01-01

### Fixed

- One.
`;

describe("parseChangelog", () => {
  it("splits the file into versions, newest first, without link definitions", () => {
    expect(parseChangelog(SAMPLE)).toEqual([
      { version: "0.2.0", date: "2026-02-01", body: "### Added\n\n- Two." },
      { version: "0.1.0", date: "2026-01-01", body: "### Fixed\n\n- One." },
    ]);
  });

  it("returns no versions for a file without version headings", () => {
    expect(parseChangelog("# Notes\n\nnothing")).toEqual([]);
  });

  it("reads the repository changelog with the running version first", () => {
    const entries = parseChangelog(changelog);
    expect(entries[0]?.version).toBe(UI_VERSION);
    expect(entries.length).toBeGreaterThan(10);
  });
});
