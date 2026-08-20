export type W3CCapabilities = Record<string, unknown>;
export interface W3CElement {
  "element-6066-11e4-a52e-4f735466cecf": string;
}

interface W3CErrorValue {
  error?: string;
  message?: string;
}

interface W3CResponse<T> {
  sessionId?: string;
  value?: T | (W3CErrorValue & { sessionId?: string });
}

const MAX_ERROR_BODY_BYTES = 16 * 1024;

function boundedBody(text: string): string {
  const body = Buffer.from(text);
  if (body.length <= MAX_ERROR_BODY_BYTES) return text;
  return `${body.subarray(body.length - MAX_ERROR_BODY_BYTES).toString()} [truncated to last ${MAX_ERROR_BODY_BYTES} bytes]`;
}

export class W3CClient {
  private sessionId: string | null = null;

  constructor(
    private readonly baseUrl: string,
    private readonly requestTimeoutMs = 10_000,
  ) {}

  async createSession(capabilities: W3CCapabilities): Promise<void> {
    const response = await this.request<{ sessionId?: string }>("POST", "/session", {
      capabilities: { alwaysMatch: capabilities, firstMatch: [{}] },
    });
    const value = response.value;
    const sessionId =
      response.sessionId ??
      (value && typeof value === "object" && "sessionId" in value ? value.sessionId : undefined);
    if (typeof sessionId !== "string" || sessionId.length === 0) {
      throw new Error(`WebDriver did not return a session id: ${JSON.stringify(response)}`);
    }
    this.sessionId = sessionId;
  }

  async deleteSession(): Promise<void> {
    if (!this.sessionId) return;
    const sessionId = this.sessionId;
    this.sessionId = null;
    await this.request("DELETE", `/session/${encodeURIComponent(sessionId)}`);
  }

  async execute<T>(script: string, args: unknown[] = []): Promise<T> {
    const response = await this.sessionRequest<T>("POST", "/execute/sync", { script, args });
    return response.value as T;
  }

  async windowHandles(): Promise<string[]> {
    const response = await this.sessionRequest<string[]>("GET", "/window/handles");
    return response.value as string[];
  }

  async switchToWindow(handle: string): Promise<void> {
    await this.sessionRequest("POST", "/window", { handle });
  }

  async findElement(using: "css selector", value: string): Promise<W3CElement> {
    const response = await this.sessionRequest<W3CElement>("POST", "/element", { using, value });
    return response.value as W3CElement;
  }

  async elementText(element: W3CElement): Promise<string> {
    const id = element["element-6066-11e4-a52e-4f735466cecf"];
    const response = await this.sessionRequest<string>(
      "GET",
      `/element/${encodeURIComponent(id)}/text`,
    );
    return response.value as string;
  }

  async elementClick(element: W3CElement): Promise<void> {
    const id = element["element-6066-11e4-a52e-4f735466cecf"];
    await this.sessionRequest(
      "POST",
      `/element/${encodeURIComponent(id)}/click`,
      {},
    );
  }

  async elementSendKeys(element: W3CElement, value: string): Promise<void> {
    const id = element["element-6066-11e4-a52e-4f735466cecf"];
    await this.sessionRequest(
      "POST",
      `/element/${encodeURIComponent(id)}/value`,
      { text: value, value: [...value] },
    );
  }

  async elementProperty(element: W3CElement, name: string): Promise<unknown> {
    const id = element["element-6066-11e4-a52e-4f735466cecf"];
    const response = await this.sessionRequest<unknown>(
      "GET",
      `/element/${encodeURIComponent(id)}/property/${encodeURIComponent(name)}`,
    );
    return response.value;
  }

  async performActions(actions: unknown[]): Promise<void> {
    await this.sessionRequest("POST", "/actions", { actions });
  }

  /** Undo every still-depressed key and button and drop the session's input
   * state. The pointer position and held keys left by one performActions call
   * survive into the next one until this is called. The leak is intra-spec
   * only: the desktop fixture is per test (fixture.ts `{ scope: "test" }`), so
   * a session never outlives the spec that opened it. */
  async releaseActions(): Promise<void> {
    await this.sessionRequest("DELETE", "/actions");
  }

  private async sessionRequest<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<W3CResponse<T>> {
    if (!this.sessionId) throw new Error("WebDriver session has not been created");
    return this.request<T>(
      method,
      `/session/${encodeURIComponent(this.sessionId)}${path}`,
      body,
    );
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<W3CResponse<T>> {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.requestTimeoutMs);
    let response: Response;
    let text: string;
    try {
      response = await fetch(`${this.baseUrl}${path}`, {
        method,
        headers: body === undefined ? undefined : { "content-type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
      });
      text = await response.text();
    } finally {
      clearTimeout(timeout);
    }

    let payload: W3CResponse<T>;
    try {
      payload = text.length === 0 ? {} : (JSON.parse(text) as W3CResponse<T>);
    } catch {
      throw new Error(`WebDriver returned invalid JSON (${response.status}): ${text}`);
    }
    if (!response.ok) {
      const value = payload.value as W3CErrorValue | undefined;
      const detail = [value?.error, value?.message].filter(Boolean).join(": ");
      throw new Error(
        `WebDriver HTTP ${response.status}${detail ? `: ${detail}` : ""}; body=${boundedBody(text)}`,
      );
    }
    return payload;
  }
}
