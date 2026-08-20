import { useEffect, useState } from "react";
import { toast } from "sonner";
import { apiGet, apiPost, apiDelete, ApiError } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

interface Rule {
  id: string; priority: number; scope: string; model_glob: string; kind: string;
  max_requests: number; max_tokens: number; window_s: number; allow: string[]; deny: string[]; route: string; enabled: boolean;
}

export default function RulesPage() {
  const [rules, setRules] = useState<Rule[]>([]);
  const [scope, setScope] = useState("global");
  const [kind, setKind] = useState("rate-limit");
  const [model, setModel] = useState("");
  const [maxReq, setMaxReq] = useState("");
  const [windowS, setWindowS] = useState("");
  const [allow, setAllow] = useState("");
  const [route, setRoute] = useState("");

  const load = () => apiGet<{ rules: Rule[] }>("/api/proxy-rules").then((r) => setRules(r.rules)).catch(() => setRules([]));
  useEffect(() => { void load(); }, []);

  const save = async () => {
    const body: Record<string, unknown> = { scope, kind, model: model || undefined };
    if (kind === "rate-limit") { body["max-requests"] = maxReq ? Number(maxReq) : undefined; body["window-s"] = windowS ? Number(windowS) : undefined; }
    else { body.allow = allow || undefined; body.route = route || undefined; }
    try { await apiPost("/api/proxy-rules", body); toast.success("rule saved"); void load(); }
    catch (e) { toast.error(`save failed: ${e instanceof ApiError ? e.message : String(e)}`); }
  };
  const remove = (id: string) =>
    apiDelete(`/api/proxy-rules/${encodeURIComponent(id)}`).then(() => { toast.success("removed"); void load(); }).catch((e) => toast.error(`rm failed: ${e instanceof ApiError ? e.message : String(e)}`));

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-lg font-semibold">Proxy rules</h1>
      <Card>
        <CardHeader className="pb-2"><CardTitle className="text-base">Add rule</CardTitle></CardHeader>
        <CardContent className="flex flex-wrap items-end gap-2">
          <Input value={scope} onChange={(e) => setScope(e.target.value)} placeholder="global | agent:x | group:g" className="h-8 w-52" />
          <select value={kind} onChange={(e) => setKind(e.target.value)} className="h-8 rounded border bg-background px-2 text-sm">
            <option value="rate-limit">rate-limit</option>
            <option value="model-policy">model-policy</option>
          </select>
          <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="model glob (opt)" className="h-8 w-40" />
          {kind === "rate-limit" ? (
            <>
              <Input value={maxReq} onChange={(e) => setMaxReq(e.target.value)} placeholder="max-requests" className="h-8 w-32" />
              <Input value={windowS} onChange={(e) => setWindowS(e.target.value)} placeholder="window-s" className="h-8 w-28" />
            </>
          ) : (
            <>
              <Input value={allow} onChange={(e) => setAllow(e.target.value)} placeholder="allow globs (csv)" className="h-8 w-40" />
              <Input value={route} onChange={(e) => setRoute(e.target.value)} placeholder="route to model" className="h-8 w-40" />
            </>
          )}
          <Button size="sm" onClick={() => void save()}>Save</Button>
        </CardContent>
      </Card>
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr><th className="px-3 py-2">Scope</th><th className="px-3 py-2">Kind</th><th className="px-3 py-2">Detail</th><th className="px-3 py-2">Enabled</th><th className="px-3 py-2"></th></tr>
          </thead>
          <tbody>
            {rules.map((r) => (
              <tr key={r.id} className="border-t">
                <td className="px-3 py-1.5 font-mono">{r.scope}</td>
                <td className="px-3 py-1.5">{r.kind}</td>
                <td className="px-3 py-1.5 text-xs text-muted-foreground">
                  {r.kind === "rate-limit" ? `${r.max_requests} req / ${r.window_s}s` : `${(r.allow || []).join(",")}${r.route ? ` → ${r.route}` : ""}`}
                </td>
                <td className="px-3 py-1.5">{r.enabled ? <Badge>on</Badge> : <Badge variant="secondary">off</Badge>}</td>
                <td className="px-3 py-1.5 text-right"><Button size="sm" variant="destructive" onClick={() => void remove(r.id)}>Remove</Button></td>
              </tr>
            ))}
            {rules.length === 0 && <tr><td colSpan={5} className="px-3 py-4 text-center text-muted-foreground">No rules.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
