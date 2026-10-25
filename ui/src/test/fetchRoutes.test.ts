import { expect, it } from "vitest";
import { routeFetch } from "./fetchRoutes";

const text = (body: string) => ({ ok: true, status: 200, text: async () => body }) as Response;

it("picks the first route whose method and path match", async () => {
  const fetch = routeFetch(
    {
      "GET /api/stores": () => text("list"),
      "POST /api/stores": () => text("create"),
      "* /api/stores/*": (req) => text(`any ${req.method} ${req.path}`),
    },
    () => text("fallback"),
  );
  expect(await (await fetch("https://host/api/stores")).text()).toBe("list");
  expect(await (await fetch("/api/stores", { method: "POST" })).text()).toBe("create");
  expect(await (await fetch("/api/stores/design", { method: "DELETE" })).text()).toBe(
    "any DELETE /api/stores/design",
  );
  expect(await (await fetch("/api/other")).text()).toBe("fallback");
});

it("parses the query and a JSON body for the handler", async () => {
  const fetch = routeFetch({}, (req) =>
    text(JSON.stringify([req.url, req.search.get("key"), req.body])),
  );
  const res = await fetch("/api/secrets?key=A", { method: "POST", body: '{"v":1}' });
  expect(JSON.parse(await res.text())).toEqual(["/api/secrets?key=A", "A", { v: 1 }]);
});

it("passes an awaited response through unchanged", async () => {
  const pending = Promise.resolve(text("later"));
  const fetch = routeFetch({ "GET /slow": () => pending }, () => text("fallback"));
  expect(await (await fetch("/slow")).text()).toBe("later");
});

it("rejects instead of throwing when a handler throws", async () => {
  const fetch = routeFetch({}, () => {
    throw new Error("unexpected request");
  });
  const result = fetch("/missing");
  await expect(result).rejects.toThrow("unexpected request");
});
