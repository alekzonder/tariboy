import { useEffect, useState } from "react";
import { apiGet } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

interface EvalRow {
  id: string; iteration: string; agent: string; image_name: string; image_tag: string;
  eval_name: string; eval_type: string; verdict: string; score: number; detail: string; created_at: string;
}

export default function EvalsPage() {
  const [rows, setRows] = useState<EvalRow[]>([]);
  useEffect(() => {
    const load = () => apiGet<{ evals: EvalRow[] }>("/api/evals").then((r) => setRows(r.evals)).catch(() => setRows([]));
    load();
    const t = window.setInterval(load, 5000);
    return () => window.clearInterval(t);
  }, []);

  const verdictBadge = (v: string) =>
    v === "pass" ? <Badge>pass</Badge> : v === "fail" ? <Badge variant="destructive">fail</Badge> : <Badge variant="secondary">{v}</Badge>;

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-lg font-semibold">Evals</h1>
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr><th className="px-3 py-2">Iteration</th><th className="px-3 py-2">Agent</th><th className="px-3 py-2">Image</th><th className="px-3 py-2">Eval</th><th className="px-3 py-2">Verdict</th><th className="px-3 py-2 text-right">Score</th></tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id} className="border-t">
                <td className="px-3 py-1.5 font-mono text-xs">{r.iteration}</td>
                <td className="px-3 py-1.5">{r.agent}</td>
                <td className="px-3 py-1.5 text-muted-foreground">{r.image_name}:{r.image_tag}</td>
                <td className="px-3 py-1.5">{r.eval_name} <span className="text-xs text-muted-foreground">({r.eval_type})</span></td>
                <td className="px-3 py-1.5">{verdictBadge(r.verdict)}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.score.toFixed(2)}</td>
              </tr>
            ))}
            {rows.length === 0 && <tr><td colSpan={6} className="px-3 py-4 text-center text-muted-foreground">No eval results.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
