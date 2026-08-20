import { createServer } from "node:http";
import type { AddressInfo } from "node:net";

import { afterEach, describe, expect, it } from "vitest";

import { W3CClient } from "./w3c";

const servers: ReturnType<typeof createServer>[] = [];

afterEach(async () => {
  await Promise.all(
    servers.splice(0).map(
      (server) => new Promise<void>((resolve) => server.close(() => resolve())),
    ),
  );
});

describe("W3CClient", () => {
  it("creates a session, executes JavaScript, and deletes the session", async () => {
    const requests: Array<{ method: string; url: string; body: unknown }> = [];
    const server = createServer((request, response) => {
      const chunks: Buffer[] = [];
      request.on("data", (chunk: Buffer) => chunks.push(chunk));
      request.on("end", () => {
        const body = chunks.length > 0 ? JSON.parse(Buffer.concat(chunks).toString()) : null;
        requests.push({ method: request.method ?? "", url: request.url ?? "", body });
        response.setHeader("content-type", "application/json");
        if (request.method === "POST" && request.url === "/session") {
          response.end(JSON.stringify({ value: { sessionId: "desktop-session", capabilities: {} } }));
          return;
        }
        if (request.method === "POST" && request.url === "/session/desktop-session/execute/sync") {
          response.end(JSON.stringify({ value: true }));
          return;
        }
        if (request.method === "DELETE" && request.url === "/session/desktop-session") {
          response.end(JSON.stringify({ value: null }));
          return;
        }
        response.statusCode = 404;
        response.end(JSON.stringify({ value: { error: "unknown command", message: "unexpected" } }));
      });
    });
    servers.push(server);
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    const client = new W3CClient(`http://127.0.0.1:${address.port}`);

    await client.createSession({ "tauri:options": { application: "/tmp/Tariboy" } });
    await expect(client.execute<boolean>("return window.__TAURI_INTERNALS__ !== undefined")).resolves.toBe(true);
    await client.deleteSession();

    expect(requests).toEqual([
      {
        method: "POST",
        url: "/session",
        body: {
          capabilities: {
            alwaysMatch: { "tauri:options": { application: "/tmp/Tariboy" } },
            firstMatch: [{}],
          },
        },
      },
      {
        method: "POST",
        url: "/session/desktop-session/execute/sync",
        body: { script: "return window.__TAURI_INTERNALS__ !== undefined", args: [] },
      },
      { method: "DELETE", url: "/session/desktop-session", body: null },
    ]);
  });

  it("includes the WebDriver error in protocol failures", async () => {
    const server = createServer((_request, response) => {
      response.statusCode = 500;
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ value: { error: "session not created", message: "bad application", driverTrace: "webkit detail" } }));
    });
    servers.push(server);
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    const client = new W3CClient(`http://127.0.0.1:${address.port}`);

    await expect(client.createSession({})).rejects.toThrow(
      'WebDriver HTTP 500: session not created: bad application; body={"value":{"error":"session not created","message":"bad application","driverTrace":"webkit detail"}}',
    );
  });

  it("switches windows and interacts with a real element", async () => {
    const requests: Array<{ method: string; url: string; body: unknown }> = [];
    const server = createServer((request, response) => {
      const chunks: Buffer[] = [];
      request.on("data", (chunk: Buffer) => chunks.push(chunk));
      request.on("end", () => {
        const body = chunks.length > 0 ? JSON.parse(Buffer.concat(chunks).toString()) : null;
        requests.push({ method: request.method ?? "", url: request.url ?? "", body });
        response.setHeader("content-type", "application/json");
        const route = `${request.method} ${request.url}`;
        const values: Record<string, unknown> = {
          "POST /session": { sessionId: "desktop-session", capabilities: {} },
          "GET /session/desktop-session/window/handles": ["main"],
          "POST /session/desktop-session/window": null,
          "POST /session/desktop-session/element": {
            "element-6066-11e4-a52e-4f735466cecf": "navigation",
          },
          "POST /session/desktop-session/element/navigation/click": null,
          "POST /session/desktop-session/element/navigation/value": null,
          "POST /session/desktop-session/actions": null,
          "GET /session/desktop-session/element/navigation/text": "Agents",
          "GET /session/desktop-session/element/navigation/property/value": "codex",
          "DELETE /session/desktop-session": null,
        };
        if (!(route in values)) {
          response.statusCode = 404;
          response.end(JSON.stringify({ value: { error: "unknown command", message: route } }));
          return;
        }
        response.end(JSON.stringify({ value: values[route] }));
      });
    });
    servers.push(server);
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    const client = new W3CClient(`http://127.0.0.1:${address.port}`);

    await client.createSession({});
    await expect(client.windowHandles()).resolves.toEqual(["main"]);
    await client.switchToWindow("main");
    const element = await client.findElement("css selector", "nav");
    await client.elementClick(element);
    await client.elementSendKeys(element, "claude");
    await expect(client.elementText(element)).resolves.toBe("Agents");
    await expect(client.elementProperty(element, "value")).resolves.toBe("codex");
    await client.performActions([{ type: "pointer", id: "mouse", actions: [{ type: "pointerDown", button: 0 }] }]);
    await client.deleteSession();

    expect(requests).toEqual([
      { method: "POST", url: "/session", body: { capabilities: { alwaysMatch: {}, firstMatch: [{}] } } },
      { method: "GET", url: "/session/desktop-session/window/handles", body: null },
      { method: "POST", url: "/session/desktop-session/window", body: { handle: "main" } },
      { method: "POST", url: "/session/desktop-session/element", body: { using: "css selector", value: "nav" } },
      { method: "POST", url: "/session/desktop-session/element/navigation/click", body: {} },
      { method: "POST", url: "/session/desktop-session/element/navigation/value", body: { text: "claude", value: ["c", "l", "a", "u", "d", "e"] } },
      { method: "GET", url: "/session/desktop-session/element/navigation/text", body: null },
      { method: "GET", url: "/session/desktop-session/element/navigation/property/value", body: null },
      {
        method: "POST",
        url: "/session/desktop-session/actions",
        body: { actions: [{ type: "pointer", id: "mouse", actions: [{ type: "pointerDown", button: 0 }] }] },
      },
      { method: "DELETE", url: "/session/desktop-session", body: null },
    ]);
  });

  it("releases input state with DELETE on the session's actions endpoint", async () => {
    const requests: Array<{ method: string; url: string }> = [];
    const server = createServer((request, response) => {
      request.on("data", () => {});
      request.on("end", () => {
        requests.push({ method: request.method ?? "", url: request.url ?? "" });
        response.setHeader("content-type", "application/json");
        if (request.method === "POST" && request.url === "/session") {
          response.end(JSON.stringify({ value: { sessionId: "desktop-session", capabilities: {} } }));
          return;
        }
        if (request.method === "DELETE" && request.url === "/session/desktop-session/actions") {
          response.end(JSON.stringify({ value: null }));
          return;
        }
        response.statusCode = 404;
        response.end(JSON.stringify({ value: { error: "unknown command", message: `${request.method} ${request.url}` } }));
      });
    });
    servers.push(server);
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    const client = new W3CClient(`http://127.0.0.1:${address.port}`);

    await client.createSession({});
    // Both halves are pinned: the verb, because a POST here would add input
    // state instead of dropping it, and the path, because /actions on the
    // session is the only endpoint that releases it. A typo in either would
    // otherwise surface only in a full desktop e2e run.
    await client.releaseActions();

    expect(requests).toEqual([
      { method: "POST", url: "/session" },
      { method: "DELETE", url: "/session/desktop-session/actions" },
    ]);
  });
});
