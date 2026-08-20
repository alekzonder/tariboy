import { useState, type FormEvent } from "react";
import { setToken, clearToken, probeAuth } from "../lib/storeApi";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";

// Login collects a bearer token, stores it (write-only: never rendered back,
// never in a URL, never logged), then probes the catalog to confirm the token is
// valid. The token lives only in sessionStorage + the masked input's transient
// state.
export default function Login({ onAuthed }: { onAuthed: () => void }) {
  const [token, setTokenState] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setError("");
    setToken(token);
    try {
      const ok = await probeAuth();
      if (ok) {
        setTokenState(""); // drop the plaintext from component state
        onAuthed();
      } else {
        clearToken();
        setError("Invalid token or insufficient scope.");
      }
    } catch (err) {
      clearToken();
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex h-screen items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Sign in to tariboy-store</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="token">Bearer token</Label>
              <Input
                id="token"
                type="password"
                autoComplete="off"
                value={token}
                onChange={(e) => setTokenState(e.target.value)}
                placeholder="paste your registry token"
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" disabled={busy || !token}>
              {busy ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
