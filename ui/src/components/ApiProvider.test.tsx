import { describe, expect, it } from "vitest";
import { render, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { ApiProvider } from "@/components/ApiProvider";
import { useApi } from "@/components/apiContext";
import { createApi } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

const host: Daemon = { id: "a", label: "A", baseURL: "https://a.example", token: "t1" };

function wrap(target: Daemon | null) {
  return ({ children }: { children: ReactNode }) => <ApiProvider target={target}>{children}</ApiProvider>;
}

describe("ApiProvider", () => {
  it("fails loudly when no provider is mounted", () => {
    expect(() => renderHook(() => useApi())).toThrow(/ApiProvider/);
  });

  it("gives children a client bound to the provider target", () => {
    const { result } = renderHook(() => useApi(), { wrapper: wrap(host) });
    expect(result.current.target).toEqual(host);
    const local = renderHook(() => useApi(), { wrapper: wrap(null) });
    expect(local.result.current.target).toBeNull();
  });

  it("keeps the same client across renders of an equal target", () => {
    const seen: unknown[] = [];
    function Probe() { seen.push(useApi()); return null; }
    const { rerender } = render(<ApiProvider target={{ ...host }}><Probe /></ApiProvider>);
    rerender(<ApiProvider target={{ ...host }}><Probe /></ApiProvider>);
    expect(seen).toHaveLength(2);
    expect(seen[0]).toBe(seen[1]);
  });

  it("replaces the client when the endpoint or token of the same host changes", () => {
    const seen: ReturnType<typeof useApi>[] = [];
    function Probe() { seen.push(useApi()); return null; }
    const { rerender } = render(<ApiProvider target={host}><Probe /></ApiProvider>);
    rerender(<ApiProvider target={{ ...host, token: "t2" }}><Probe /></ApiProvider>);
    rerender(<ApiProvider target={{ ...host, token: "t2", baseURL: "https://b.example" }}><Probe /></ApiProvider>);
    expect(seen[1]).not.toBe(seen[0]);
    expect(seen[1].target?.token).toBe("t2");
    expect(seen[2].target?.baseURL).toBe("https://b.example");
  });

  it("accepts a prepared client, for tests and multi-host views", () => {
    const client = createApi(host);
    const { result } = renderHook(() => useApi(), {
      wrapper: ({ children }) => <ApiProvider client={client}>{children}</ApiProvider>,
    });
    expect(result.current).toBe(client);
  });
});
