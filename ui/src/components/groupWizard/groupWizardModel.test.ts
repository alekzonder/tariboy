import { describe, expect, it } from "vitest";
import {
  canCreate,
  emptyDraft,
  groupWizardReducer,
  initialWizardState,
  specFor,
  type AgentDraft,
  type WizardState,
} from "./groupWizardModel";

const draft = (id: number, up: Partial<AgentDraft> = {}): AgentDraft => ({
  ...emptyDraft(id),
  ...up,
});

const withRows = (count: number, leaderIdx: number): WizardState => ({
  ...initialWizardState(),
  agents: Array.from({ length: count }, (_, i) => draft(i)),
  nextId: count,
  leaderIdx,
});

describe("groupWizardReducer", () => {
  it("starts with one empty row led by it", () => {
    expect(initialWizardState()).toEqual({
      agents: [emptyDraft(0)],
      nextId: 1,
      leaderIdx: 0,
      rowStatus: {},
    });
  });

  it("adds a row with the next stable id", () => {
    const next = groupWizardReducer(initialWizardState(), { type: "addAgent" });
    expect(next.agents.map((a) => a.id)).toEqual([0, 1]);
    expect(next.agents[1]).toEqual(emptyDraft(1));
    expect(next.nextId).toBe(2);
  });

  it.each([
    ["before the leader", 0, 2, 1],
    ["the leader itself", 2, 2, 0],
    ["after the leader", 3, 1, 1],
  ])("removes a row %s and keeps the leader on a valid row", (_, idx, leader, want) => {
    const next = groupWizardReducer(withRows(4, leader), { type: "removeAgent", id: idx, idx });
    expect(next.agents.map((a) => a.id)).toEqual([0, 1, 2, 3].filter((id) => id !== idx));
    expect(next.leaderIdx).toBe(want);
    expect(next.nextId).toBe(4);
  });

  it("patches only the row with the given id", () => {
    const next = groupWizardReducer(withRows(2, 0), {
      type: "patchAgent",
      id: 1,
      patch: { name: "bob", interactive: true },
    });
    expect(next.agents[0]).toEqual(draft(0));
    expect(next.agents[1]).toEqual(draft(1, { name: "bob", interactive: true }));
  });

  it("moves the leader marker", () => {
    expect(groupWizardReducer(withRows(3, 0), { type: "setLeader", idx: 2 }).leaderIdx).toBe(2);
  });

  it("records per-row statuses by draft id", () => {
    let state = groupWizardReducer(initialWizardState(), {
      type: "setRowStatus",
      id: 0,
      status: { kind: "creating" },
    });
    state = groupWizardReducer(state, {
      type: "setRowStatus",
      id: 5,
      status: { kind: "failed", reason: "boom" },
    });
    state = groupWizardReducer(state, {
      type: "setRowStatus",
      id: 0,
      status: { kind: "created", name: "alice" },
    });
    expect(state.rowStatus).toEqual({
      0: { kind: "created", name: "alice" },
      5: { kind: "failed", reason: "boom" },
    });
  });
});

describe("specFor", () => {
  const full = draft(3, {
    image: "img:1",
    name: " alice ",
    cwd: " /own ",
    model: " opus ",
    effort: "high",
    harness: " claude ",
    timeout: " 60m ",
    interactive: true,
    envRows: [{ key: "A", value: "1" }],
    plugins: ["p1", "p2"],
  });

  it("trims fields, inherits the base cwd and drops plugins when not allowed", () => {
    expect(specFor(full, " /base ", " team ", false)).toEqual({
      image: "img:1",
      name: "alice",
      cwd: "/base",
      model: "opus",
      effort: "high",
      harness: "claude",
      interactive: true,
      env: "A=1",
      plugins: undefined,
      timeout: "60m",
      group: "team",
    });
  });

  it("uses the row cwd on override and joins allowed plugins", () => {
    const spec = specFor({ ...full, override: true }, "/base", "team", true);
    expect(spec.cwd).toBe("/own");
    expect(spec.plugins).toBe("p1,p2");
  });

  it("leaves blank optional fields undefined", () => {
    expect(specFor(draft(0, { image: "img:2" }), "", "team", true)).toEqual({
      image: "img:2",
      name: undefined,
      cwd: undefined,
      model: undefined,
      effort: undefined,
      harness: undefined,
      interactive: false,
      env: undefined,
      plugins: undefined,
      timeout: undefined,
      group: "team",
    });
  });
});

describe("canCreate", () => {
  const rows = [draft(0, { image: "a", name: "lead" }), draft(1, { image: "b" })];

  it("needs a group name, a named leader and an image on every row", () => {
    expect(canCreate("team", rows, 0)).toBe(true);
    expect(canCreate("  ", rows, 0)).toBe(false);
    expect(canCreate("team", rows, 1)).toBe(false);
    expect(canCreate("team", [rows[0], draft(1)], 0)).toBe(false);
    expect(canCreate("team", [], 0)).toBe(false);
  });
});
