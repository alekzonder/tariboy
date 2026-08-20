import { useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import {
  getTags, getManifest,
  type PushRow, type StoreManifest,
} from "../lib/storeApi";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export default function RepoDetail() {
  const { name = "" } = useParams();
  const [rows, setRows] = useState<PushRow[] | null>(null);
  const [tag, setTag] = useState<string>("");
  const [manifest, setManifest] = useState<StoreManifest | null>(null);
  const [error, setError] = useState("");
  // Monotonic request token: guards against out-of-order getManifest responses
  // (e.g. clicking tag A then tag B, where A's fetch resolves after B's) and
  // against setManifest firing after unmount. Only the response whose token
  // still matches the latest dispatched request is applied.
  const reqRef = useRef(0);

  // selectTag drives the manifest fetch directly (from the initial tags load
  // and from a row click) rather than via a tag-keyed effect — chaining the
  // manifest request onto the same call site keeps it a single hop behind
  // the version-history render instead of stacking an extra effect-flush cycle.
  function selectTag(t: string) {
    setTag(t);
    setError("");
    const myReq = ++reqRef.current;
    setManifest(null);
    getManifest(name, t)
      .then((m) => { if (myReq === reqRef.current) setManifest(m); })
      .catch(() => { if (myReq === reqRef.current) setManifest(null); });
  }

  // Bumping the token on unmount invalidates any in-flight request so its
  // resolution/rejection is a no-op instead of calling setManifest on an
  // unmounted component.
  useEffect(() => () => { reqRef.current++; }, []);

  useEffect(() => {
    let alive = true;
    getTags(name)
      .then((t) => {
        if (!alive) return;
        setError("");
        setRows(t.tags);
        if (t.tags.length > 0) selectTag(t.tags[0].tag);
      })
      .catch((e: unknown) => alive && setError(e instanceof Error ? e.message : String(e)));
    return () => {
      alive = false;
    };
    // selectTag is a plain function redefined on every render that closes
    // over the current `name`/`reqRef`; it is not a stable dep, and adding it
    // here would refetch tags every render, so `name` alone is correct.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name]);

  const pullCmd = useMemo(
    () => `tariboy pull ${name}:${tag || "latest"} --registry ${window.location.origin}`,
    [name, tag],
  );

  if (error) return <p className="p-6 text-sm text-destructive">{error}</p>;
  if (!rows) return <p className="p-6 text-sm text-muted-foreground">Loading…</p>;

  return (
    <div className="flex flex-col gap-4 p-6">
      <h1 className="text-xl font-semibold">{name}</h1>

      <Card>
        <CardHeader>
          <CardTitle>Pull</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center gap-2">
          <code className="flex-1 overflow-x-auto rounded bg-muted px-2 py-1 text-sm">{pullCmd}</code>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => navigator.clipboard?.writeText(pullCmd)}
          >
            Copy
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Version history</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground">
                  <th className="py-1 pr-4">tag</th>
                  <th className="py-1 pr-4">digest</th>
                  <th className="py-1 pr-4">built</th>
                  <th className="py-1">pushed</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r, i) => (
                  <tr
                    key={`${r.tag}-${r.digest}-${i}`}
                    className="cursor-pointer border-t hover:bg-accent"
                    onClick={() => selectTag(r.tag)}
                  >
                    <td className="py-1 pr-4">
                      {r.tag === tag ? <Badge>{r.tag}</Badge> : r.tag}
                    </td>
                    <td className="py-1 pr-4 font-mono text-xs">{r.digest}</td>
                    <td className="py-1 pr-4">{r.built_at}</td>
                    <td className="py-1">{r.pushed_at}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {manifest && (
        <Card>
          <CardHeader>
            <CardTitle>Manifest — {name}:{tag}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            <div>
              <span className="text-muted-foreground">harness: </span>
              {manifest.harness.type}
              {manifest.harness.model ? ` (${manifest.harness.model})` : ""}
            </div>
            <div>
              <span className="text-muted-foreground">plugins: </span>
              {manifest.plugins.length === 0
                ? "none"
                : manifest.plugins.map((p) => `${p.name}${p.version ? " " + p.version : ""}`).join(", ")}
            </div>
            <div>
              <span className="text-muted-foreground">requires_secrets: </span>
              {manifest.requires_secrets.length === 0 ? "none" : manifest.requires_secrets.join(", ")}
            </div>
            <div>
              <span className="text-muted-foreground">evals: </span>
              {manifest.evals.length === 0 ? "none" : manifest.evals.map((e) => e.name).join(", ")}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
