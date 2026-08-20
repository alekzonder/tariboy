import { useEffect, useState } from "react";
import { toast } from "sonner";
import { apiGet, apiPost, ApiError } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

interface StatusRow { scope: string; limit_usd: number; spent_usd: number; mode: string; over: boolean }

export default function BudgetsPage() {
  const [rows, setRows] = useState<StatusRow[]>([]);
  const [scope, setScope] = useState("global");
  const [limit, setLimit] = useState("");
  const [period, setPeriod] = useState("24h");
  const [mode, setMode] = useState("warn");

  const load = () => apiGet<{ budgets: StatusRow[] }>("/api/budgets/status").then((r) => setRows(r.budgets)).catch(() => setRows([]));
  useEffect(() => { void load(); }, []);

  const save = async () => {
    try {
      await apiPost("/api/budgets", { scope, "limit-usd": limit, period, mode });
      toast.success("budget saved");
      setLimit("");
      void load();
    } catch (e) { toast.error(`save failed: ${e instanceof ApiError ? e.message : String(e)}`); }
  };

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-lg font-semibold">Budgets</h1>
      <Card>
        <CardHeader className="pb-2"><CardTitle className="text-base">Set budget</CardTitle></CardHeader>
        <CardContent className="flex flex-wrap items-end gap-2">
          <Input value={scope} onChange={(e) => setScope(e.target.value)} placeholder="global | agent:x | group:g" className="h-8 w-56" />
          <Input value={limit} onChange={(e) => setLimit(e.target.value)} placeholder="limit USD" className="h-8 w-28" />
          <Input value={period} onChange={(e) => setPeriod(e.target.value)} placeholder="period (24h)" className="h-8 w-28" />
          <select value={mode} onChange={(e) => setMode(e.target.value)} className="h-8 rounded border bg-background px-2 text-sm">
            <option value="warn">warn</option>
            <option value="block">block</option>
          </select>
          <Button size="sm" onClick={() => void save()}>Save</Button>
        </CardContent>
      </Card>
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr><th className="px-3 py-2">Scope</th><th className="px-3 py-2 text-right">Limit</th><th className="px-3 py-2 text-right">Spent</th><th className="px-3 py-2">Mode</th><th className="px-3 py-2">Status</th></tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i} className="border-t">
                <td className="px-3 py-1.5 font-mono">{r.scope}</td>
                <td className="px-3 py-1.5 text-right font-mono">${r.limit_usd.toFixed(2)}</td>
                <td className="px-3 py-1.5 text-right font-mono">${r.spent_usd.toFixed(4)}</td>
                <td className="px-3 py-1.5">{r.mode}</td>
                <td className="px-3 py-1.5">{r.over ? <Badge variant="destructive">over</Badge> : <Badge variant="secondary">ok</Badge>}</td>
              </tr>
            ))}
            {rows.length === 0 && <tr><td colSpan={5} className="px-3 py-4 text-center text-muted-foreground">No budgets.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
