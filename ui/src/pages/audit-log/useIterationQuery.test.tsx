import type { PropsWithChildren } from "react";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MemoryRouter, useLocation, useNavigationType } from "react-router-dom";
import { useIterationQuery } from "./useIterationQuery";

function setup(url: string, period?: { since: string; until: string }) {
  const wrapper = ({ children }: PropsWithChildren) => (
    <MemoryRouter initialEntries={[url]}>{children}</MemoryRouter>
  );
  return renderHook(
    () => ({ q: useIterationQuery(period), location: useLocation(), nav: useNavigationType() }),
    { wrapper },
  );
}

describe("useIterationQuery", () => {
  it("builds the daemon query from the URL filters", () => {
    const { result } = setup("/?tag=a,b&started_after=2026-01-01&started_before=2026-02-01");
    expect(result.current.q.tagFilter).toBe("a,b");
    expect(result.current.q.afterFilter).toBe("2026-01-01");
    expect(result.current.q.beforeFilter).toBe("2026-02-01");
    expect(result.current.q.queryString).toBe(
      "tag=a%2Cb&started_after=2026-01-01&started_before=2026-02-01",
    );
    expect(result.current.q.selected).toBeNull();
  });

  it("lets the period replace the started-at filters", () => {
    const { result } = setup("/?started_after=2026-01-01", { since: "S", until: "U" });
    expect(result.current.q.queryString).toBe("started_after=S&started_before=U");
  });

  it("replaces the history entry for filters and pushes for selection", () => {
    const { result } = setup("/?iteration=x");
    expect(result.current.q.selected).toBe("x");
    act(() => result.current.q.setFilter("tag", "t"));
    expect(result.current.nav).toBe("REPLACE");
    expect(result.current.location.search).toBe("?iteration=x&tag=t");
    act(() => result.current.q.select(null));
    expect(result.current.nav).toBe("PUSH");
    expect(result.current.location.search).toBe("?tag=t");
    act(() => result.current.q.setFilter("tag", ""));
    expect(result.current.location.search).toBe("");
    act(() => result.current.q.select("y"));
    expect(result.current.q.selected).toBe("y");
  });
});
