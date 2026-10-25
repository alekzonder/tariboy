// routeFetch builds a fake `fetch` from a table of "METHOD /path" routes, so a
// test's fake server is a list of small handlers instead of one long if-chain.
// METHOD may be `*`; a path ending in `*` matches by prefix. The first matching
// route in insertion order wins; nothing matching goes to `fallback`.

export interface FetchRequest {
  url: string;
  method: string;
  path: string;
  search: URLSearchParams;
  // The parsed JSON body, typed like JSON.parse returns it.
  body: ReturnType<typeof JSON.parse>;
  init: RequestInit;
}

type Handler = (req: FetchRequest) => Response | Promise<Response>;

function matches(route: string, req: FetchRequest) {
  const [method, path] = route.split(" ");
  if (method !== "*" && method !== req.method) return false;
  return path.endsWith("*") ? req.path.startsWith(path.slice(0, -1)) : req.path === path;
}

export function routeFetch(routes: Record<string, Handler>, fallback: Handler) {
  return (input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> => {
    const url = String(input);
    const parsed = new URL(url, "http://local");
    const req: FetchRequest = {
      url,
      method: init.method ?? "GET",
      path: parsed.pathname,
      search: parsed.searchParams,
      body: typeof init.body === "string" ? JSON.parse(init.body) : undefined,
      init,
    };
    const route = Object.keys(routes).find((key) => matches(key, req));
    return Promise.resolve((route ? routes[route] : fallback)(req));
  };
}
