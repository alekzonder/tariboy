import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { listRepos, StoreApiError, type Repo } from "../lib/storeApi";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export default function Catalog() {
  const [repos, setRepos] = useState<Repo[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    listRepos()
      .then((c) => alive && setRepos(c.repos))
      .catch((e: unknown) => alive && setError(e instanceof StoreApiError ? e.message : String(e)));
    return () => {
      alive = false;
    };
  }, []);

  if (error) return <p className="p-6 text-sm text-destructive">{error}</p>;
  if (!repos) return <p className="p-6 text-sm text-muted-foreground">Loading…</p>;
  if (repos.length === 0)
    return <p className="p-6 text-sm text-muted-foreground">No repositories yet. Push an image with <code>tariboy push</code>.</p>;

  return (
    <div className="grid gap-3 p-6 sm:grid-cols-2 lg:grid-cols-3">
      {repos.map((r) => (
        <Link key={r.name} to={`/repo/${encodeURIComponent(r.name)}`}>
          <Card className="transition-colors hover:border-primary">
            <CardHeader>
              <CardTitle className="truncate">{r.name}</CardTitle>
            </CardHeader>
            <CardContent>
              <Badge variant="secondary">{r.tags.length} tags</Badge>
            </CardContent>
          </Card>
        </Link>
      ))}
    </div>
  );
}
