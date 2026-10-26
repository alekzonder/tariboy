import { afterEach, expect, it, vi } from "vitest";
import type { Daemon } from "../daemons";
import { createApi } from ".";

afterEach(() => vi.restoreAllMocks());

const target: Daemon = { id: "h", label: "H", baseURL: "https://h.example", token: "t" };

function stubFetch() {
  const calls: Array<{ method: string; url: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({
        method: init.method ?? "GET",
        url,
        body: init.body ? JSON.parse(init.body as string) : undefined,
      });
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result: {} }),
      } as Response;
    }),
  );
  return calls;
}

it("addresses every mail route and acts as the given principal", async () => {
  const calls = stubFetch();
  const { mail } = createApi(target);
  await mail.list({ box: "archive", as: "agent:dev", before: "2026-10-26T00:00:00Z" });
  await mail.list({ box: "inbox" });
  await mail.show("m 1", "agent:dev");
  await mail.send({ to: ["agent:dev"], subject: "Hi", text: "Body", notice: true });
  await mail.reply("m1", { text: "Ok", as: "agent:dev" });
  await mail.close("m1", { note: "done" });
  await mail.read(["m1", "m2"], "agent:dev");
  await mail.requeue("m1", "agent:dev");
  await mail.unread();
  await mail.clear("dev");
  const base = "https://h.example";
  expect(calls).toEqual([
    {
      method: "GET",
      url: `${base}/api/mail?box=archive&as=agent%3Adev&before=2026-10-26T00%3A00%3A00Z`,
      body: undefined,
    },
    { method: "GET", url: `${base}/api/mail?box=inbox`, body: undefined },
    { method: "GET", url: `${base}/api/mail/m%201?as=agent%3Adev`, body: undefined },
    {
      method: "POST",
      url: `${base}/api/mail`,
      body: { to: ["agent:dev"], subject: "Hi", text: "Body", notice: true },
    },
    { method: "POST", url: `${base}/api/mail/m1/reply`, body: { text: "Ok", as: "agent:dev" } },
    { method: "POST", url: `${base}/api/mail/m1/close`, body: { note: "done" } },
    { method: "POST", url: `${base}/api/mail/read`, body: { ids: ["m1", "m2"], as: "agent:dev" } },
    { method: "POST", url: `${base}/api/mail/m1/requeue`, body: { as: "agent:dev" } },
    { method: "GET", url: `${base}/api/mail/unread`, body: undefined },
    { method: "POST", url: `${base}/api/agents/dev/mail/clear`, body: undefined },
  ]);
});
