import { describe, expect, it } from "vitest";
import {
  cardPosition,
  compareVersions,
  TOUR_ANCHORS,
  TOUR_STEPS,
  tourMode,
  tourSteps,
  whatsNewSteps,
  type TourStep,
} from "./tour";
import { readKey, storageKeyFamilies, storageKeys, writeKey } from "@/lib/storage";
import { navPath } from "@/lib/navRoutes";

const step = (id: string, since?: string, updated?: boolean): TourStep => ({
  id,
  title: id,
  body: id,
  since,
  updated,
});

describe("compareVersions", () => {
  it("orders by number, not by text", () => {
    expect(compareVersions("0.10.0", "0.9.9")).toBeGreaterThan(0);
    expect(compareVersions("0.97.0", "0.97.0")).toBe(0);
    expect(compareVersions("0.97.0", "1.0.0")).toBeLessThan(0);
    expect(compareVersions("0.97.1", "0.97.0")).toBeGreaterThan(0);
  });
});

describe("tourMode", () => {
  const steps = [step("base"), step("queues", "0.99.0")];

  it("starts nothing on the first run", () => {
    expect(tourMode("", "0.99.0", steps)).toBe("none");
  });

  it("runs What's new when a step changed after the saved version", () => {
    expect(tourMode("0.98.0", "0.99.0", steps)).toBe("whats-new");
  });

  it("runs nothing when no step changed after the saved version", () => {
    expect(tourMode("0.98.0", "0.98.5", steps)).toBe("none");
    expect(tourMode("0.99.0", "0.99.0", steps)).toBe("none");
  });

  it("ignores a step newer than the running version", () => {
    expect(tourMode("0.98.0", "0.98.1", [step("later", "1.0.0")])).toBe("none");
  });
});

describe("whatsNewSteps", () => {
  it("keeps the steps changed in (saved, current]", () => {
    const steps = [step("base"), step("old", "0.90.0"), step("new", "0.99.0"), step("x", "1.0.0")];
    expect(whatsNewSteps(steps, "0.98.0", "0.99.0").map((s) => s.id)).toEqual(["new"]);
  });
});

describe("the onboarding storage key", () => {
  it("keeps a version and reads anything else as the first run", () => {
    expect(writeKey(storageKeys.onboarding, "0.97.0")).toBe(true);
    expect(readKey(storageKeys.onboarding)).toBe("0.97.0");
    localStorage.setItem(storageKeys.onboarding.name, "garbage");
    expect(readKey(storageKeys.onboarding)).toBe("");
  });
});

describe("TOUR_STEPS", () => {
  it("anchors every step to a declared anchor and names each id once", () => {
    const anchors = new Set<string>(Object.values(TOUR_ANCHORS));
    for (const s of TOUR_STEPS) {
      if (s.anchor) expect(anchors.has(s.anchor)).toBe(true);
    }
    expect(new Set(TOUR_STEPS.map((s) => s.id)).size).toBe(TOUR_STEPS.length);
  });

  it("goes through the work process in the design order", () => {
    expect(TOUR_STEPS.map((s) => s.id)).toEqual([
      "welcome",
      "agents",
      "agent-workspace",
      "agent-settings",
      "tasks",
      "images",
      "stores",
      "servers",
      "add-server",
      "queues",
      "workflows",
      "help",
    ]);
  });

  it("points the Agents step at the home New agent button, not the sidebar +", () => {
    const agents = TOUR_STEPS.find((s) => s.id === "agents");
    expect(agents).toMatchObject({ anchor: TOUR_ANCHORS.newAgent, home: true });
    expect(agents?.sidebar).toBeFalsy();
  });

  it("shows the steps whose place moved to the sidebar in What's new", () => {
    expect(TOUR_STEPS.filter((s) => s.since !== undefined).map((s) => s.id)).toEqual([
      "tasks",
      "images",
      "stores",
      "servers",
      "add-server",
      "queues",
      "workflows",
    ]);
    for (const s of TOUR_STEPS.filter((s) => s.since !== undefined)) {
      expect(s).toMatchObject({ since: "0.101.0", updated: true });
    }
  });

  it("shows Add host on Settings → Hosts, not in the sidebar", () => {
    const addHost = TOUR_STEPS.find((s) => s.id === "add-server");
    expect(addHost).toMatchObject({ anchor: TOUR_ANCHORS.addHost, route: "settings" });
    expect(addHost?.sidebar).toBeFalsy();
    expect(addHost?.body).toContain("Settings → Hosts");
  });

  it("opens every step's page through navPath", () => {
    for (const s of TOUR_STEPS.filter((s) => s.route)) {
      expect(navPath(s.route!, "")).toMatch(/^\//);
    }
  });

  it("puts every anchor a step names on an element of some component", () => {
    const sources = Object.entries(
      import.meta.glob<string>(["/src/**/*.tsx", "!/src/**/*.test.tsx"], {
        query: "?raw",
        import: "default",
        eager: true,
      }),
    );
    const keyOf = new Map(Object.entries(TOUR_ANCHORS).map(([key, value]) => [value, key]));
    for (const s of TOUR_STEPS.filter((s) => s.anchor)) {
      const key = keyOf.get(s.anchor!);
      const used = sources.some(([, text]) => text.includes(`TOUR_ANCHORS.${key}`));
      expect(used, `${s.id}: TOUR_ANCHORS.${key}`).toBe(true);
    }
  });

  it("covers the whole working process", () => {
    const titles = TOUR_STEPS.map((s) => s.title).join(" ");
    for (const topic of [
      "Host",
      "image",
      "Sources",
      "Agent",
      "Tasks",
      "Queues",
      "workflow",
      "settings",
    ]) {
      expect(titles).toContain(topic);
    }
  });
});

describe("tourSteps", () => {
  it("drops the agent steps when there is no agent, so the counter counts the rest", () => {
    const steps = tourSteps(TOUR_STEPS, false);
    expect(steps).toHaveLength(10);
    expect(steps.some((s) => s.agent)).toBe(false);
    expect(tourSteps(TOUR_STEPS, true)).toHaveLength(TOUR_STEPS.length);
  });
});

describe("the hint storage family", () => {
  it("keeps one dismissal per host, agent and tab", () => {
    const key = storageKeyFamilies.onboardingHint.key("h1", "dev", "tasks");
    expect(key.name).toBe("onboarding:hint:v1:h1:dev:tasks");
    expect(readKey(key)).toBe(false);
    expect(writeKey(key, true)).toBe(true);
    expect(readKey(key)).toBe(true);
    expect(readKey(storageKeyFamilies.onboardingHint.key("h1", "other", "tasks"))).toBe(false);
  });
});

describe("cardPosition", () => {
  const viewport = { width: 1000, height: 800 };
  const card = { width: 300, height: 150 };

  it("centers the card without a target", () => {
    expect(cardPosition(null, viewport, card)).toEqual({ top: 325, left: 350 });
  });

  it("puts the card to the right of a sidebar element", () => {
    const target = { top: 100, left: 10, right: 200, bottom: 130, width: 190, height: 30 };
    expect(cardPosition(target, viewport, card)).toEqual({ top: 100, left: 212 });
  });

  it("puts the card below an element at the right edge", () => {
    const target = { top: 10, left: 900, right: 990, bottom: 40, width: 90, height: 30 };
    expect(cardPosition(target, viewport, card)).toEqual({ top: 52, left: 688 });
  });

  it("puts the card to the left of an element at the bottom right corner", () => {
    const target = { top: 760, left: 900, right: 990, bottom: 790, width: 90, height: 30 };
    expect(cardPosition(target, viewport, card)).toEqual({ top: 638, left: 588 });
  });

  it("puts the card above a wide element at the bottom", () => {
    const target = { top: 760, left: 0, right: 1000, bottom: 790, width: 1000, height: 30 };
    expect(cardPosition(target, viewport, card)).toEqual({ top: 598, left: 12 });
  });

  it("centers the card when the element fills the window", () => {
    const target = { top: 0, left: 0, right: 1000, bottom: 800, width: 1000, height: 800 };
    expect(cardPosition(target, viewport, card)).toEqual({ top: 325, left: 350 });
  });
});
