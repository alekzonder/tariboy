import { afterEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import UsagePage from "./UsagePage";

function report(group = "") {
  const alpha = group === "alpha";
  const ungrouped = group === "__ungrouped__";
  const label = alpha ? "alpha" : ungrouped ? "" : "alpha";
  const requests = alpha ? 2 : ungrouped ? 1 : 3;
  const input = alpha ? 120 : ungrouped ? 30 : 150;
  const cost = alpha ? 0.012 : ungrouped ? 0.003 : 0.015;
  return {
    rows: [{
      agent: alpha ? "alice" : ungrouped ? "legacy" : "alice",
      image: "basic:latest",
      group_id: label,
      group_name: label,
      requests,
      input_tokens: input,
      output_tokens: 60,
      cache_write_tokens: 10,
      cache_read_tokens: 5,
      cost_usd: cost,
    }],
    count: 1,
    total_requests: requests,
    total_cost_usd: cost,
    total_input_tokens: input,
    total_output_tokens: 60,
    total_cache_write_tokens: 10,
    total_cache_read_tokens: 5,
    series: [{ bucket_start: "2026-08-18T00:00:00Z", requests, tokens: input + 75, cost_usd: cost }],
    requests: [{
      id: `request-${group || "all"}`,
      ts: "2026-08-18T12:30:00Z",
      agent: alpha ? "alice" : ungrouped ? "legacy" : "alice",
      image: "basic:latest",
      provider: "openai",
      model: alpha ? "gpt-alpha" : ungrouped ? "gpt-legacy" : "gpt-all",
      input_tokens: input,
      output_tokens: 60,
      cache_write_tokens: 10,
      cache_read_tokens: 5,
      cost_usd: cost,
      status: 200,
      group_id: label,
      group_name: label,
    }, ...(group ? [] : [{
      id: "request-ungrouped",
      ts: "2026-08-18T11:30:00Z",
      agent: "legacy",
      image: "basic:latest",
      provider: "openai",
      model: "gpt-null",
      input_tokens: 30,
      output_tokens: 0,
      cache_write_tokens: 0,
      cache_read_tokens: 0,
      cost_usd: 0.003,
      status: 200,
      group_id: "",
      group_name: "",
    }])],
  };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("synchronizes every server Usage projection with the selected group snapshot", async () => {
  const paths: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const path = String(input);
    paths.push(path);
    const url = new URL(path, "http://tariboy.test");
    const result = url.pathname === "/api/groups"
      ? { groups: [{ name: "alpha", lead: "alice", members: 1 }, { name: "beta", lead: "bob", members: 1 }], count: 2 }
      : report(url.searchParams.get("group") ?? "");
    return {
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ ok: true, result }),
    } as Response;
  }));

  render(<MemoryRouter><UsagePage /></MemoryRouter>);

  expect(await screen.findByText("gpt-all")).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "All groups" })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "Ungrouped" })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "alpha" })).toBeInTheDocument();
  expect(screen.getByText("gpt-null")).toBeInTheDocument();
  expect(screen.getAllByText("alpha").length).toBeGreaterThanOrEqual(2);
  expect(screen.getAllByText("Ungrouped").length).toBeGreaterThanOrEqual(2);
  expect(screen.getByLabelText("Daily usage")).toHaveTextContent("3 requests, 225 tokens, $0.0150");
  expect(screen.getByLabelText("Usage summary")).toHaveTextContent(/Requests\s*3/);

  fireEvent.change(screen.getByLabelText("Group"), { target: { value: "alpha" } });

  await waitFor(() => expect(paths.some((path) => path.includes("/api/usage?group=alpha"))).toBe(true));
  expect(await screen.findByText("gpt-alpha")).toBeInTheDocument();
  expect(screen.getByLabelText("Daily usage")).toHaveTextContent("2 requests, 195 tokens, $0.0120");
  expect(screen.getByLabelText("Usage summary")).toHaveTextContent(/Requests\s*2/);
  expect(screen.getByRole("region", { name: "Usage by agent" })).toHaveTextContent("120");
  expect(screen.getByRole("region", { name: "Usage by agent" })).not.toHaveTextContent("150");
  expect(screen.getByRole("region", { name: "Recent requests" })).toHaveTextContent("gpt-alpha");
  expect(screen.queryByText("gpt-all")).not.toBeInTheDocument();

  fireEvent.change(screen.getByLabelText("Group"), { target: { value: "__ungrouped__" } });

  await waitFor(() => expect(paths.some((path) => path.includes("/api/usage?group=__ungrouped__"))).toBe(true));
  expect(await screen.findByText("gpt-legacy")).toBeInTheDocument();
  expect(screen.getAllByText("Ungrouped").length).toBeGreaterThanOrEqual(2);
});

it("keeps the selected group Usage snapshot when an older request resolves last", async () => {
  const pending: Array<{ path: string; resolve: (response: Response) => void }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const path = String(input);
    if (path.includes("/api/groups")) {
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result: { groups: [{ name: "alpha", lead: "alice", members: 1 }], count: 1 } }),
      } as Response);
    }
    return new Promise<Response>((resolve) => pending.push({ path, resolve }));
  }));

  render(<MemoryRouter><UsagePage /></MemoryRouter>);

  await waitFor(() => expect(pending.some((request) => request.path.endsWith("/api/usage"))).toBe(true));
  fireEvent.change(screen.getByLabelText("Group"), { target: { value: "alpha" } });
  await waitFor(() => expect(pending.some((request) => request.path.endsWith("/api/usage?group=alpha"))).toBe(true));

  const response = (result: ReturnType<typeof report>) => ({
    ok: true,
    status: 200,
    text: async () => JSON.stringify({ ok: true, result }),
  } as Response);
  pending.find((request) => request.path.endsWith("/api/usage?group=alpha"))!.resolve(response(report("alpha")));
  expect(await screen.findByText("gpt-alpha")).toBeInTheDocument();

  await act(async () => {
    pending
      .filter((request) => request.path.endsWith("/api/usage"))
      .forEach((request) => request.resolve(response(report())));
  });

  expect(screen.getByLabelText("Group")).toHaveValue("alpha");
  expect(screen.getByLabelText("Usage summary")).toHaveTextContent(/Requests\s*2/);
  expect(screen.getByLabelText("Daily usage")).toHaveTextContent("2 requests, 195 tokens, $0.0120");
  expect(screen.getByRole("region", { name: "Usage by agent" })).toHaveTextContent("120");
  expect(screen.getByRole("region", { name: "Recent requests" })).toHaveTextContent("gpt-alpha");
  expect(screen.queryByText("gpt-all")).not.toBeInTheDocument();
});
